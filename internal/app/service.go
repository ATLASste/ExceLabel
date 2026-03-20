package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"excelabel/internal/domain"
	"excelabel/internal/infrastructure/fswatch"
	"excelabel/internal/infrastructure/logging"
	"excelabel/internal/infrastructure/scanner"
	"excelabel/internal/infrastructure/store"
	"excelabel/internal/infrastructure/workbook"
)

type Config struct {
	RootDir            string `json:"rootDir"`
	WorkbookPath       string `json:"workbookPath"`
	AutoSyncEnabled    bool   `json:"autoSyncEnabled"`
	WorkbookDebounceMs int    `json:"workbookDebounceMs"`
	FsDebounceMs       int    `json:"fsDebounceMs"`
	MaxScanWorkers     int    `json:"maxScanWorkers"`
}

type StatusSummary struct {
	WorkspaceReady bool                    `json:"workspaceReady"`
	WatcherActive  bool                    `json:"watcherActive"`
	Snapshot       int64                   `json:"snapshot"`
	FileCount      int                     `json:"fileCount"`
	Conflicts      []domain.ConflictResult `json:"conflicts"`
	Logs           []logging.Entry         `json:"logs"`
}

type SyncService struct {
	config    Config
	scanner   *scanner.Scanner
	workbook  *workbook.Repository
	store     *store.StateStore
	logger    *logging.Logger
	watcher   *fswatch.Watcher
	state     domain.WorkspaceState
	conflicts []domain.ConflictResult
}

func NewSyncService(config Config) *SyncService {
	config = withDefaults(config)
	statePath := store.DefaultStatePath(config.RootDir)
	return &SyncService{
		config:    config,
		scanner:   scanner.New(config.MaxScanWorkers),
		workbook:  workbook.New(),
		store:     store.New(statePath),
		logger:    logging.NewLogger(),
		conflicts: make([]domain.ConflictResult, 0),
	}
}

func (service *SyncService) StartWorkspace() error {
	if err := validateConfig(service.config); err != nil {
		return err
	}

	snapshotVersion := time.Now().UnixNano()
	entries, err := service.scanner.ScanFull(service.config.RootDir, snapshotVersion)
	if err != nil {
		return err
	}

	entries = service.excludeWorkbook(entries)
	service.state = scanner.BuildState(entries, service.config.RootDir, service.config.WorkbookPath, snapshotVersion)
	rows := domain.SnapshotRows(service.state.Entries, snapshotVersion)
	if err := service.workbook.CreateWorkbook(service.config.WorkbookPath, rows, service.state); err != nil {
		return err
	}
	if err := service.store.SaveWorkspace(service.state); err != nil {
		return err
	}

	service.conflicts = nil
	service.logger.Info("app", fmt.Sprintf("工作区初始化完成，共导出 %d 个文件", len(entries)))
	if service.config.AutoSyncEnabled {
		if err := service.StartWatching(); err != nil {
			return err
		}
	}
	return nil
}

func (service *SyncService) StartWatching() error {
	if service.watcher != nil {
		return nil
	}

	watcher, err := fswatch.New(
		service.config.RootDir,
		service.config.WorkbookPath,
		time.Duration(service.config.FsDebounceMs)*time.Millisecond,
		time.Duration(service.config.WorkbookDebounceMs)*time.Millisecond,
	)
	if err != nil {
		return err
	}
	if err := watcher.WatchRoot(service.config.RootDir); err != nil {
		return err
	}
	if err := watcher.WatchWorkbook(service.config.WorkbookPath); err != nil {
		return err
	}

	service.watcher = watcher
	service.logger.Info("watcher", "监听器已启动")
	go service.consumeEvents()
	return nil
}

func (service *SyncService) StopWatching() error {
	if service.watcher == nil {
		return nil
	}
	if err := service.watcher.Close(); err != nil {
		return err
	}
	service.watcher = nil
	service.logger.Info("watcher", "监听器已停止")
	return nil
}

func (service *SyncService) RefreshNow() error {
	service.logger.Info("app", "执行手动刷新")
	return service.StartWorkspace()
}

func (service *SyncService) ApplyWorkbookChanges() error {
	rows, err := service.workbook.LoadRows(service.config.WorkbookPath)
	if err != nil {
		return err
	}

	diff := domain.DetectWorkbookDiff(service.state.Entries, rows)
	service.conflicts = diff.Conflicts
	if len(diff.RenameRequests) == 0 {
		service.logger.Info("sync", "未检测到 Excel 改名请求")
		return nil
	}

	statusUpdates := make([]domain.WorkbookRow, 0, len(diff.RenameRequests)+len(diff.Conflicts))
	for _, request := range diff.RenameRequests {
		if err := os.Rename(request.OldPath, request.TargetPath); err != nil {
			request.Row.Status = string(domain.StatusError) + ": " + err.Error()
		} else {
			request.Row.Status = string(domain.StatusSynced)
			request.Row.LastKnownPath = domain.BuildRelativePath(request.Entry.RelativeDir, request.Row.NameStem, request.Entry.Ext)
			request.Row.RowVersion = time.Now().UnixNano()
			request.Row.LastOpSource = string(domain.EventSourceWorkbook)
			updatedEntry := request.Entry
			updatedEntry.NameStem = request.Row.NameStem
			updatedEntry.AbsolutePath = request.TargetPath
			updatedEntry.LastSeenVersion = request.Row.RowVersion
			service.state.Entries[updatedEntry.RecordID] = updatedEntry
		}
		statusUpdates = append(statusUpdates, request.Row)
	}

	for _, conflict := range diff.Conflicts {
		statusUpdates = append(statusUpdates, domain.WorkbookRow{
			RecordID: conflict.RecordID,
			Status:   string(domain.StatusConflict) + ": " + conflict.Reason,
			RowIndex: findRowIndex(rows, conflict.RecordID),
		})
	}

	if len(statusUpdates) > 0 {
		if err := service.workbook.WriteStatuses(service.config.WorkbookPath, statusUpdates); err != nil {
			if isWorkbookInUseError(err) {
				service.logger.Info("sync", "工作簿当前正被 Excel 占用，已跳过状态回写；文件改名结果已应用到磁盘")
			} else {
				return err
			}
		}
	}
	service.state.SnapshotVersion = time.Now().UnixNano()
	service.state.LastScanAt = time.Now()
	service.state.PendingEvents = nil
	if err := service.store.SaveWorkspace(service.state); err != nil {
		return err
	}
	service.logger.Info("sync", fmt.Sprintf("已处理 %d 个 Excel 改名请求", len(diff.RenameRequests)))
	return nil
}

func (service *SyncService) ApplyFilesystemChanges() error {
	if service.state.RootDir == "" {
		return fmt.Errorf("工作区尚未初始化")
	}

	snapshotVersion := time.Now().UnixNano()
	scannedEntries, err := service.scanner.ScanFull(service.config.RootDir, snapshotVersion)
	if err != nil {
		return err
	}
	scannedEntries = service.excludeWorkbook(scannedEntries)

	nextEntries := reconcileEntries(service.state.Entries, scannedEntries, snapshotVersion)
	rows := domain.SnapshotRows(nextEntries, snapshotVersion)
	if err := service.workbook.UpdateRows(service.config.WorkbookPath, rows, domain.WorkspaceState{
		RootDir:         service.state.RootDir,
		WorkbookPath:    service.state.WorkbookPath,
		SnapshotVersion: snapshotVersion,
		LastScanAt:      time.Now(),
		Entries:         nextEntries,
		PendingEvents:   nil,
	}); err != nil {
		if isWorkbookInUseError(err) {
			service.logger.Info("sync", "工作簿当前正被 Excel 占用，已跳过本次回写；关闭 Excel 后可再执行刷新")
			return nil
		}
		return err
	}

	service.state.Entries = nextEntries
	service.state.SnapshotVersion = snapshotVersion
	service.state.LastScanAt = time.Now()
	service.state.PendingEvents = nil
	service.conflicts = nil
	if err := service.store.SaveWorkspace(service.state); err != nil {
		return err
	}

	service.logger.Info("sync", fmt.Sprintf("文件系统增量对账完成，当前快照包含 %d 个文件", len(nextEntries)))
	return nil
}

func (service *SyncService) GetStatusSummary() StatusSummary {
	return StatusSummary{
		WorkspaceReady: service.state.RootDir != "",
		WatcherActive:  service.watcher != nil,
		Snapshot:       service.state.SnapshotVersion,
		FileCount:      len(service.state.Entries),
		Conflicts:      service.conflicts,
		Logs:           service.logger.Snapshot(),
	}
}

func (service *SyncService) consumeEvents() {
	if service.watcher == nil {
		return
	}
	for event := range service.watcher.Events() {
		switch event.Source {
		case domain.EventSourceWorkbook:
			if err := service.ApplyWorkbookChanges(); err != nil {
				service.logger.Error("sync", err.Error())
			}
		case domain.EventSourceFilesystem:
			service.state.PendingEvents = append(service.state.PendingEvents, event)
			if err := service.ApplyFilesystemChanges(); err != nil {
				service.logger.Error("sync", err.Error())
			}
		}
	}
}

func isWorkbookInUseError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "used by another process") ||
		strings.Contains(message, "cannot access the file") ||
		strings.Contains(message, "being used by another process")
}

func (service *SyncService) excludeWorkbook(entries []domain.FileEntry) []domain.FileEntry {
	filtered := make([]domain.FileEntry, 0, len(entries))
	workbookPath := filepath.Clean(service.config.WorkbookPath)
	for _, entry := range entries {
		if filepath.Clean(entry.AbsolutePath) == workbookPath {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func reconcileEntries(previous map[string]domain.FileEntry, current []domain.FileEntry, snapshotVersion int64) map[string]domain.FileEntry {
	nextEntries := make(map[string]domain.FileEntry, len(current))
	matchedPrevious := make(map[string]bool, len(previous))
	previousByFingerprint := make(map[string][]domain.FileEntry)
	previousByPath := make(map[string]domain.FileEntry)

	for recordID, entry := range previous {
		previousByFingerprint[entry.Fingerprint] = append(previousByFingerprint[entry.Fingerprint], entry)
		previousByPath[strings.ToLower(entry.LogicalPath())] = entry
		matchedPrevious[recordID] = false
	}

	for _, entry := range current {
		logicalPathKey := strings.ToLower(entry.LogicalPath())
		if previousEntry, exists := previousByPath[logicalPathKey]; exists && !matchedPrevious[previousEntry.RecordID] {
			entry.RecordID = previousEntry.RecordID
			entry.LastSeenVersion = snapshotVersion
			nextEntries[entry.RecordID] = entry
			matchedPrevious[entry.RecordID] = true
			continue
		}

		candidates := previousByFingerprint[entry.Fingerprint]
		matched := false
		for _, candidate := range candidates {
			if matchedPrevious[candidate.RecordID] {
				continue
			}
			entry.RecordID = candidate.RecordID
			entry.LastSeenVersion = snapshotVersion
			nextEntries[entry.RecordID] = entry
			matchedPrevious[candidate.RecordID] = true
			matched = true
			break
		}
		if matched {
			continue
		}

		entry.LastSeenVersion = snapshotVersion
		nextEntries[entry.RecordID] = entry
	}

	for recordID, entry := range previous {
		if matchedPrevious[recordID] {
			continue
		}
		entry.Exists = false
		entry.LastSeenVersion = snapshotVersion
		nextEntries[recordID] = entry
	}

	return nextEntries
}

func withDefaults(config Config) Config {
	if config.WorkbookDebounceMs <= 0 {
		config.WorkbookDebounceMs = 500
	}
	if config.FsDebounceMs <= 0 {
		config.FsDebounceMs = 800
	}
	if config.MaxScanWorkers <= 0 {
		config.MaxScanWorkers = 4
	}
	return config
}

func validateConfig(config Config) error {
	if config.RootDir == "" {
		return fmt.Errorf("根目录不能为空")
	}
	if config.WorkbookPath == "" {
		return fmt.Errorf("工作簿路径不能为空")
	}
	rootInfo, err := os.Stat(config.RootDir)
	if err != nil {
		return fmt.Errorf("根目录不可访问: %w", err)
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("根目录必须是文件夹")
	}
	if err := os.MkdirAll(filepath.Dir(config.WorkbookPath), 0o755); err != nil {
		return fmt.Errorf("创建工作簿目录失败: %w", err)
	}
	return nil
}

func findRowIndex(rows []domain.WorkbookRow, recordID string) int {
	for _, row := range rows {
		if row.RecordID == recordID {
			return row.RowIndex
		}
	}
	return 0
}
