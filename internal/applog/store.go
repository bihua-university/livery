package applog

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

const (
	defaultMaxFileBytes int64 = 2 << 20 // 2 MiB per log file
	defaultTailLimit    int   = 400
)

// Store persists application and core logs to a bounded rotating file on disk
// so log history does not stay resident in RAM.
type Store struct {
	mu       sync.Mutex
	dir      string
	path     string
	maxBytes int64
	file     *os.File
	size     int64
	rev      atomic.Uint64
}

// NewStore opens or creates a disk-backed log store in dir.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "livery.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	var size int64
	if st, err := f.Stat(); err == nil {
		size = st.Size()
	}
	return &Store{
		dir:      dir,
		path:     p,
		maxBytes: defaultMaxFileBytes,
		file:     f,
		size:     size,
	}, nil
}

// Path returns the active log file path.
func (s *Store) Path() string { return s.path }

// Dir returns the directory holding log files.
func (s *Store) Dir() string { return s.dir }

// Revision increments whenever a log entry is appended or cleared.
func (s *Store) Revision() uint64 { return s.rev.Load() }

var logMsgReplacer = strings.NewReplacer("\r", "", "\n", " ", "\t", " ")

// Append writes a single log entry directly to disk without retaining it in memory.
func (s *Store) Append(entry model.LogEntry) {
	ts := entry.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	lvl := strings.ToLower(strings.TrimSpace(entry.Level))
	if lvl == "" {
		lvl = "info"
	}
	src := strings.TrimSpace(entry.Source)
	if src == "" {
		src = "app"
	}
	msg := logMsgReplacer.Replace(strings.TrimSpace(entry.Message))
	if msg == "" {
		return
	}
	line := fmt.Sprintf("%d\t%s\t%s\t%s\n", ts.UnixMilli(), lvl, src, msg)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return
	}
	if s.size+int64(len(line)) > s.maxBytes {
		s.rotateLocked()
	}
	n, err := s.file.WriteString(line)
	if err == nil {
		s.size += int64(n)
		s.rev.Add(1)
	}
}

func (s *Store) rotateLocked() {
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	backup := s.path + ".1"
	_ = os.Remove(backup)
	_ = os.Rename(s.path, backup)
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err == nil {
		s.file = f
		s.size = 0
	}
}

// Clear truncates the active log file and removes rotated backups.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	_ = os.Remove(s.path + ".1")
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	s.file = f
	s.size = 0
	s.rev.Add(1)
	return nil
}

// Close closes the underlying log file handle.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		return err
	}
	return nil
}

// Tail reads up to limit matching log entries from the end of the log file on demand.
func (s *Store) Tail(limit int, levelFilter, query string) ([]model.LogEntry, error) {
	if limit <= 0 {
		limit = defaultTailLimit
	}
	wantLevel := strings.ToLower(strings.TrimSpace(levelFilter))
	q := strings.ToLower(strings.TrimSpace(query))

	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return nil, err
	}

	const chunkSize int64 = 32 * 1024
	offset := st.Size()
	var carry []byte
	var readBuf []byte
	collected := make([]model.LogEntry, 0, min(limit, 64))

	for offset > 0 && len(collected) < limit {
		readSize := chunkSize
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize

		need := int(readSize) + len(carry)
		if cap(readBuf) < need {
			readBuf = make([]byte, need)
		} else {
			readBuf = readBuf[:need]
		}
		if _, err := f.ReadAt(readBuf[:readSize], offset); err != nil && err != io.EOF {
			return nil, err
		}
		if len(carry) > 0 {
			copy(readBuf[readSize:], carry)
		}

		// Find first newline so partial leading line carries to the next earlier chunk.
		buf := readBuf
		if offset > 0 {
			firstNL := bytes.IndexByte(buf, '\n')
			if firstNL < 0 {
				carry = append(carry[:0], buf...)
				continue
			}
			carry = append(carry[:0], buf[:firstNL]...)
			buf = buf[firstNL+1:]
		} else {
			carry = carry[:0]
		}

		for len(buf) > 0 && len(collected) < limit {
			nl := bytes.LastIndexByte(buf, '\n')
			var line []byte
			if nl >= 0 {
				line = buf[nl+1:]
				buf = buf[:nl]
			} else {
				line = buf
				buf = nil
			}
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			entry, ok := parseLineFiltered(line, wantLevel, q)
			if !ok {
				continue
			}
			collected = append(collected, entry)
		}
	}

	// Reverse into chronological order.
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return collected, nil
}

func parseLineFiltered(line []byte, wantLevel, q string) (model.LogEntry, bool) {
	t1 := bytes.IndexByte(line, '\t')
	if t1 <= 0 {
		return model.LogEntry{}, false
	}
	rest := line[t1+1:]
	t2 := bytes.IndexByte(rest, '\t')
	if t2 < 0 {
		return model.LogEntry{}, false
	}
	lvlBytes := rest[:t2]
	if wantLevel != "" && !bytes.HasPrefix(lvlBytes, []byte(wantLevel)) {
		return model.LogEntry{}, false
	}
	rest = rest[t2+1:]
	t3 := bytes.IndexByte(rest, '\t')
	if t3 < 0 {
		return model.LogEntry{}, false
	}
	srcBytes := rest[:t3]
	msgBytes := rest[t3+1:]
	if q != "" && !strings.Contains(strings.ToLower(string(msgBytes)), q) {
		return model.LogEntry{}, false
	}
	ms, err := strconv.ParseInt(string(line[:t1]), 10, 64)
	if err != nil {
		return model.LogEntry{}, false
	}
	return model.LogEntry{
		Time:    time.UnixMilli(ms),
		Level:   string(lvlBytes),
		Source:  string(srcBytes),
		Message: string(msgBytes),
	}, true
}
