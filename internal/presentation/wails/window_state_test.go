package wails

import (
	"path/filepath"
	"testing"
)

func TestClampRectToAreaKeepsWindowInsideWorkArea(t *testing.T) {
	area := windowRect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	cases := []struct {
		name string
		in   windowRect
		want windowRect
	}{
		{"inside unchanged", windowRect{100, 100, 900, 700}, windowRect{100, 100, 900, 700}},
		{"title bar above screen", windowRect{100, -200, 900, 400}, windowRect{100, 0, 900, 600}},
		{"off right and bottom", windowRect{1800, 900, 2600, 1500}, windowRect{1120, 440, 1920, 1040}},
		{"larger than screen", windowRect{-50, -50, 2500, 1600}, windowRect{0, 0, 1920, 1040}},
		{"on disconnected monitor left", windowRect{-2000, 100, -1200, 700}, windowRect{0, 100, 800, 700}},
	}
	for _, tc := range cases {
		if got := clampRectToArea(tc.in, area); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestWindowStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "window-state.json")
	if _, ok := loadWindowState(path); ok {
		t.Fatal("missing file should not load")
	}
	want := windowState{Normal: windowRect{10, 20, 1010, 820}, Maximized: true}
	if err := saveWindowState(path, want); err != nil {
		t.Fatal(err)
	}
	got, ok := loadWindowState(path)
	if !ok || got != want {
		t.Fatalf("got %+v ok=%v, want %+v", got, ok, want)
	}
}
