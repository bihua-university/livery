package applog

import (
	"fmt"
	"testing"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

func TestStoreTailFilterAndRotate(t *testing.T) {
	dir := t.TempDir()
	st, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer st.Close()

	for i := range 20 {
		lvl := "info"
		if i%5 == 0 {
			lvl = "error"
		}
		st.Append(model.LogEntry{
			Time:    time.Now(),
			Level:   lvl,
			Source:  "sing-box",
			Message: fmt.Sprintf("line-%02d", i),
		})
	}

	tail, err := st.Tail(5, "", "")
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(tail) != 5 || tail[0].Message != "line-15" || tail[4].Message != "line-19" {
		t.Fatalf("unexpected tail: %+v", tail)
	}

	errs, err := st.Tail(10, "error", "")
	if err != nil {
		t.Fatalf("Tail error: %v", err)
	}
	if len(errs) != 4 {
		t.Fatalf("expected 4 error lines, got %d", len(errs))
	}

	if err := st.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	empty, err := st.Tail(10, "", "")
	if err != nil || len(empty) != 0 {
		t.Fatalf("expected empty after Clear, got %+v (err=%v)", empty, err)
	}
}
