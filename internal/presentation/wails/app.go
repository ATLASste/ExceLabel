package wails

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"excelabel/internal/app"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type DesktopApp struct {
	ctx     context.Context
	service *app.SyncService
}

func NewDesktopApp(service *app.SyncService) *DesktopApp {
	return &DesktopApp{service: service}
}

func (desktop *DesktopApp) Startup(ctx context.Context) {
	desktop.ctx = ctx
}

func (desktop *DesktopApp) InitializeWorkspace(rootDir, workbookPath string) (app.StatusSummary, error) {
	desktop.service = app.NewSyncService(app.Config{
		RootDir:         rootDir,
		WorkbookPath:    workbookPath,
		AutoSyncEnabled: true,
	})
	if err := desktop.service.StartWorkspace(); err != nil {
		return app.StatusSummary{}, err
	}
	return desktop.service.GetStatusSummary(), nil
}

func (desktop *DesktopApp) RefreshNow() (app.StatusSummary, error) {
	if desktop.service == nil {
		return app.StatusSummary{}, nil
	}
	if err := desktop.service.RefreshNow(); err != nil {
		return app.StatusSummary{}, err
	}
	return desktop.service.GetStatusSummary(), nil
}

func (desktop *DesktopApp) ApplyWorkbookChanges() (app.StatusSummary, error) {
	if desktop.service == nil {
		return app.StatusSummary{}, nil
	}
	if err := desktop.service.ApplyWorkbookChanges(); err != nil {
		return app.StatusSummary{}, err
	}
	return desktop.service.GetStatusSummary(), nil
}

func (desktop *DesktopApp) StartWatching() (app.StatusSummary, error) {
	if desktop.service == nil {
		return app.StatusSummary{}, nil
	}
	if err := desktop.service.StartWatching(); err != nil {
		return app.StatusSummary{}, err
	}
	return desktop.service.GetStatusSummary(), nil
}

func (desktop *DesktopApp) StopWatching() (app.StatusSummary, error) {
	if desktop.service == nil {
		return app.StatusSummary{}, nil
	}
	if err := desktop.service.StopWatching(); err != nil {
		return app.StatusSummary{}, err
	}
	return desktop.service.GetStatusSummary(), nil
}

func (desktop *DesktopApp) GetStatusSummary() app.StatusSummary {
	if desktop.service == nil {
		return app.StatusSummary{}
	}
	return desktop.service.GetStatusSummary()
}

func (desktop *DesktopApp) SelectRootDirectory(defaultDirectory string) (string, error) {
	return runtime.OpenDirectoryDialog(desktop.ctx, runtime.OpenDialogOptions{
		Title:                "选择需要扫描的根目录",
		DefaultDirectory:     normalizeDirectory(defaultDirectory),
		CanCreateDirectories: true,
	})
}

func (desktop *DesktopApp) SelectWorkbookSavePath(defaultPath string) (string, error) {
	defaultDirectory, defaultFilename := normalizeWorkbookPath(defaultPath)
	return runtime.SaveFileDialog(desktop.ctx, runtime.SaveDialogOptions{
		Title:                "选择工作簿保存位置",
		DefaultDirectory:     defaultDirectory,
		DefaultFilename:      defaultFilename,
		CanCreateDirectories: true,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Excel 工作簿 (*.xlsx)",
				Pattern:     "*.xlsx",
			},
		},
	})
}

func (desktop *DesktopApp) SelectExistingWorkbookPath(defaultPath string) (string, error) {
	defaultDirectory, _ := normalizeWorkbookPath(defaultPath)
	return runtime.OpenFileDialog(desktop.ctx, runtime.OpenDialogOptions{
		Title:                "选择现有工作簿",
		DefaultDirectory:     defaultDirectory,
		CanCreateDirectories: true,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Excel 工作簿 (*.xlsx)",
				Pattern:     "*.xlsx",
			},
		},
	})
}

func (desktop *DesktopApp) OpenWorkbook(workbookPath string) error {
	cleaned := strings.TrimSpace(workbookPath)
	if cleaned == "" {
		return fmt.Errorf("工作簿路径不能为空")
	}

	cleaned = filepath.Clean(cleaned)
	if _, err := os.Stat(cleaned); err != nil {
		return fmt.Errorf("工作簿不可访问: %w", err)
	}

	cmd := exec.Command("cmd", "/c", "start", "", cleaned)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开工作簿失败: %w", err)
	}
	return nil
}

func normalizeDirectory(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	cleaned := filepath.Clean(path)
	if filepath.Ext(cleaned) != "" {
		return filepath.Dir(cleaned)
	}
	return cleaned
}

func normalizeWorkbookPath(path string) (string, string) {
	if strings.TrimSpace(path) == "" {
		return "", "excelabel.xlsx"
	}

	cleaned := filepath.Clean(path)
	defaultDirectory := filepath.Dir(cleaned)
	defaultFilename := filepath.Base(cleaned)
	if defaultFilename == "." || defaultFilename == string(filepath.Separator) || defaultFilename == "" {
		defaultFilename = "excelabel.xlsx"
	}
	if !strings.EqualFold(filepath.Ext(defaultFilename), ".xlsx") {
		defaultFilename += ".xlsx"
	}
	return defaultDirectory, defaultFilename
}
