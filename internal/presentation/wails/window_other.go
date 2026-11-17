//go:build !windows

package wails

func restoreWindowPlacement(*windowState) bool { return false }

func currentWindowPlacement() (windowState, bool) { return windowState{}, false }
