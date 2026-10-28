//go:build !windows

package term

func prepareConsole() bool { return true }
