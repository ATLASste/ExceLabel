//go:build windows

package wails

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	swHide                = 0
	swShowMinimized       = 2
	swShowMaximized       = 3
	wpfRestoreToMaximized = 2
	monitorDefaultNearest = 2
	spiGetWorkArea        = 0x0030
	wailsWindowClassName  = "wailsWindow"
	classNameBufferLength = 256
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procGetWindowPlacement       = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement       = user32.NewProc("SetWindowPlacement")
	procMonitorFromRect          = user32.NewProc("MonitorFromRect")
	procGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	procSystemParametersInfoW    = user32.NewProc("SystemParametersInfoW")
)

type win32Point struct{ X, Y int32 }

type windowPlacement struct {
	Length           uint32
	Flags            uint32
	ShowCmd          uint32
	PtMinPosition    win32Point
	PtMaxPosition    win32Point
	RcNormalPosition windowRect
}

type monitorInfo struct {
	CbSize    uint32
	RcMonitor windowRect
	RcWork    windowRect
	DwFlags   uint32
}

func findMainWindow() uintptr {
	pid := uint32(os.Getpid())
	var found uintptr
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var windowPID uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if windowPID != pid {
			return 1
		}
		buffer := make([]uint16, classNameBufferLength)
		length, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		if length > 0 && syscall.UTF16ToString(buffer[:length]) == wailsWindowClassName {
			found = hwnd
			return 0
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	return found
}

func getPlacement(hwnd uintptr) (windowPlacement, bool) {
	placement := windowPlacement{}
	placement.Length = uint32(unsafe.Sizeof(placement))
	ok, _, _ := procGetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&placement)))
	return placement, ok != 0
}

// workspaceOrigin returns the screen position of workspace coordinate (0,0),
// which is the top-left of the primary monitor's work area.
func workspaceOrigin() (int32, int32) {
	var area windowRect
	if ok, _, _ := procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&area)), 0); ok == 0 {
		return 0, 0
	}
	return area.Left, area.Top
}

// fitToMonitor clamps a rectangle given in workspace coordinates to the work
// area of the nearest monitor.
func fitToMonitor(rect windowRect) windowRect {
	originX, originY := workspaceOrigin()
	screenRect := windowRect{Left: rect.Left + originX, Top: rect.Top + originY, Right: rect.Right + originX, Bottom: rect.Bottom + originY}
	monitor, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&screenRect)), monitorDefaultNearest)
	if monitor == 0 {
		return rect
	}
	info := monitorInfo{}
	info.CbSize = uint32(unsafe.Sizeof(info))
	if ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return rect
	}
	fitted := clampRectToArea(screenRect, info.RcWork)
	return windowRect{Left: fitted.Left - originX, Top: fitted.Top - originY, Right: fitted.Right - originX, Bottom: fitted.Bottom - originY}
}

// restoreWindowPlacement applies the saved restore rectangle (or the default
// one) after fitting it on screen, keeping the window hidden.
func restoreWindowPlacement(saved *windowState) bool {
	hwnd := findMainWindow()
	if hwnd == 0 {
		return false
	}
	placement, ok := getPlacement(hwnd)
	if !ok {
		return false
	}
	if saved != nil {
		placement.RcNormalPosition = saved.Normal
	}
	placement.RcNormalPosition = fitToMonitor(placement.RcNormalPosition)
	placement.Flags = 0
	placement.ShowCmd = swHide
	applied, _, _ := procSetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&placement)))
	return applied != 0
}

func currentWindowPlacement() (windowState, bool) {
	hwnd := findMainWindow()
	if hwnd == 0 {
		return windowState{}, false
	}
	placement, ok := getPlacement(hwnd)
	if !ok || placement.RcNormalPosition.width() <= 0 || placement.RcNormalPosition.height() <= 0 {
		return windowState{}, false
	}
	maximized := placement.ShowCmd == swShowMaximized ||
		(placement.ShowCmd == swShowMinimized && placement.Flags&wpfRestoreToMaximized != 0)
	return windowState{Normal: placement.RcNormalPosition, Maximized: maximized}, true
}
