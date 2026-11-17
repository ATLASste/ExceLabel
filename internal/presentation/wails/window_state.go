package wails

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	MinWindowWidth  = 960
	MinWindowHeight = 600
)

// windowRect is a window rectangle in physical pixels.
type windowRect struct {
	Left   int32 `json:"left"`
	Top    int32 `json:"top"`
	Right  int32 `json:"right"`
	Bottom int32 `json:"bottom"`
}

func (r windowRect) width() int32  { return r.Right - r.Left }
func (r windowRect) height() int32 { return r.Bottom - r.Top }

type windowState struct {
	Normal    windowRect `json:"normal"`
	Maximized bool       `json:"maximized"`
}

// clampRectToArea shrinks the rectangle to fit inside the area and moves it
// inside, so the title bar is always reachable.
func clampRectToArea(rect, area windowRect) windowRect {
	width := min(rect.width(), area.width())
	height := min(rect.height(), area.height())
	left := min(max(rect.Left, area.Left), area.Right-width)
	top := min(max(rect.Top, area.Top), area.Bottom-height)
	return windowRect{Left: left, Top: top, Right: left + width, Bottom: top + height}
}

func windowStatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ExceLabel", "window-state.json"), nil
}

func loadWindowState(path string) (windowState, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return windowState{}, false
	}
	var state windowState
	if err := json.Unmarshal(content, &state); err != nil || state.Normal.width() <= 0 || state.Normal.height() <= 0 {
		return windowState{}, false
	}
	return state, true
}

func saveWindowState(path string, state windowState) error {
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

// DomReady restores the last window placement (or maximizes on first launch)
// and then shows the window, which starts hidden.
func (desktop *DesktopApp) DomReady(ctx context.Context) {
	desktop.showOnce.Do(func() {
		maximize := true
		var saved *windowState
		if path, err := windowStatePath(); err == nil {
			if state, ok := loadWindowState(path); ok {
				saved = &state
				maximize = state.Maximized
			}
		}
		if !restoreWindowPlacement(saved) && !maximize {
			runtime.WindowCenter(ctx)
		}
		if maximize {
			runtime.WindowMaximise(ctx)
		}
		runtime.WindowShow(ctx)
	})
}

// BeforeClose saves the window placement; it never blocks closing.
func (desktop *DesktopApp) BeforeClose(ctx context.Context) bool {
	state, ok := currentWindowPlacement()
	if !ok {
		return false
	}
	if path, err := windowStatePath(); err == nil {
		_ = saveWindowState(path, state)
	}
	return false
}
