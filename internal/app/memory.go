package app

import (
	"runtime/metrics"
	"strconv"
	"strings"
)

// sampleAppMemory returns the current process resident set size (RSS) in bytes,
// falling back to Go runtime mapped memory if OS RSS is unavailable.
func sampleAppMemory() int64 {
	if rss := sampleProcessRSS(); rss > 0 {
		return rss
	}
	return sampleRuntimeMemory()
}

func sampleRuntimeMemory() int64 {
	var samples [3]metrics.Sample
	samples[0].Name = "/memory/classes/total:bytes"
	samples[1].Name = "/memory/classes/heap/released:bytes"
	samples[2].Name = "/memory/classes/heap/objects:bytes"
	metrics.Read(samples[:])

	var total, released, objects uint64
	if samples[0].Value.Kind() == metrics.KindUint64 {
		total = samples[0].Value.Uint64()
	}
	if samples[1].Value.Kind() == metrics.KindUint64 {
		released = samples[1].Value.Uint64()
	}
	if samples[2].Value.Kind() == metrics.KindUint64 {
		objects = samples[2].Value.Uint64()
	}
	if total > released {
		return int64(total - released)
	}
	return int64(objects)
}

func parseProcStatmRSS(data []byte, pageSize int64) int64 {
	if pageSize <= 0 {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || pages <= 0 {
		return 0
	}
	return pages * pageSize
}

func parsePSRssKiB(data []byte) int64 {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return 0
	}
	kib, err := strconv.ParseInt(s, 10, 64)
	if err != nil || kib <= 0 {
		return 0
	}
	return kib * 1024
}
