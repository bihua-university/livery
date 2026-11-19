//go:build linux

package app

import "os"

func sampleProcessRSS() int64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	return parseProcStatmRSS(data, int64(os.Getpagesize()))
}
