//go:build !windows && !linux && !darwin

package app

func sampleProcessPrivateMemory(int) int64 {
	return 0
}

func findCoreListenerPID(int, string) int {
	return 0
}

func trimProcessMemory() {}

