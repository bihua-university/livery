//go:build darwin

package app

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func sampleProcessRSS() int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		return 0
	}
	return parsePSRssKiB(out)
}
