package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type EventSource string

type EventType string

type RowStatus string

const (
	EventSourceWorkbook   EventSource = "workbook"
	EventSourceFilesystem EventSource = "filesystem"
	EventSourceInternal   EventSource = "internal"
)

const (
	EventTypeCreated  EventType = "created"
	EventTypeUpdated  EventType = "updated"
	EventTypeDeleted  EventType = "deleted"
	EventTypeRenamed  EventType = "renamed"
	EventTypeUnknown  EventType = "unknown"
)

const (
	StatusSynced             RowStatus = "✅ 同步成功"
	StatusPendingExcelChange RowStatus = "⏳ 待应用 Excel 修改"
	StatusPendingFsChange    RowStatus = "⏳ 待处理文件系统变化"
	StatusApplyingRename     RowStatus = "🔄 正在执行重命名"
	StatusConflict           RowStatus = "❌ 冲突"
	StatusError              RowStatus = "❌ 错误"
	StatusDeleted            RowStatus = "🗑 已删除"
	StatusMetadataCorrupted  RowStatus = "❌ 元数据异常"
)

var windowsIllegalChars = `\\/:*?"<>|`

type FileEntry struct {
	RecordID        string    `json:"recordId"`
	NameStem        string    `json:"nameStem"`
	Ext             string    `json:"ext"`
	RelativeDir     string    `json:"relativeDir"`
	AbsolutePath    string    `json:"absolutePath"`
	Fingerprint     string    `json:"fingerprint"`
	Size            int64     `json:"size"`
	ModTime         time.Time `json:"modTime"`
	Exists          bool      `json:"exists"`
	LastSeenVersion int64     `json:"lastSeenVersion"`
}

func (entry FileEntry) LogicalPath() string {
	return BuildRelativePath(entry.RelativeDir, entry.NameStem, entry.Ext)
}

func (entry FileEntry) CurrentStatus() string {
	if !entry.Exists {
		return string(StatusDeleted)
	}
	return string(StatusSynced)
}

func (entry FileEntry) AbsoluteTarget(rootDir string) string {
	return filepath.Join(rootDir, filepath.FromSlash(entry.LogicalPath()))
}

type WorkbookRow struct {
	RowIndex       int    `json:"rowIndex"`
	RecordID       string `json:"recordId"`
	NameStem       string `json:"nameStem"`
	Ext            string `json:"ext"`
	RelativeDir    string `json:"relativeDir"`
	Status         string `json:"status"`
	Fingerprint    string `json:"fingerprint"`
	LastKnownPath  string `json:"lastKnownPath"`
	RowVersion     int64  `json:"rowVersion"`
	Tombstone      bool   `json:"tombstone"`
	LastOpSource   string `json:"lastOpSource"`
}

type SyncEvent struct {
	EventID        string      `json:"eventId"`
	Source         EventSource `json:"source"`
	Type           EventType   `json:"type"`
	RecordID       string      `json:"recordId"`
	OldPath        string      `json:"oldPath"`
	NewPath        string      `json:"newPath"`
	OccurredAt     time.Time   `json:"occurredAt"`
	CorrelationKey string      `json:"correlationKey"`
}

type ConflictResult struct {
	RecordID     string `json:"recordId"`
	ConflictType string `json:"conflictType"`
	TargetPath   string `json:"targetPath"`
	Reason       string `json:"reason"`
	Suggestion   string `json:"suggestion"`
}

type WorkspaceState struct {
	RootDir          string               `json:"rootDir"`
	WorkbookPath     string               `json:"workbookPath"`
	SnapshotVersion  int64                `json:"snapshotVersion"`
	LastScanAt       time.Time            `json:"lastScanAt"`
	LastWorkbookHash string               `json:"lastWorkbookHash"`
	Entries          map[string]FileEntry `json:"entries"`
	PendingEvents    []SyncEvent          `json:"pendingEvents"`
}

type RenameRequest struct {
	RecordID   string      `json:"recordId"`
	OldPath    string      `json:"oldPath"`
	TargetPath string      `json:"targetPath"`
	Entry      FileEntry   `json:"entry"`
	Row        WorkbookRow `json:"row"`
}

type WorkbookDiff struct {
	RenameRequests []RenameRequest  `json:"renameRequests"`
	Conflicts      []ConflictResult `json:"conflicts"`
	MissingRows    []WorkbookRow    `json:"missingRows"`
}

func BuildRelativePath(relativeDir, nameStem, ext string) string {
	cleanDir := strings.Trim(strings.ReplaceAll(relativeDir, "\\", "/"), "/")
	fileName := nameStem + ext
	if cleanDir == "" {
		return fileName
	}
	return cleanDir + "/" + fileName
}

func NormalizeRelativeDir(relativeDir string) string {
	cleanDir := strings.Trim(strings.ReplaceAll(relativeDir, "\\", "/"), "/")
	if cleanDir == "." {
		return ""
	}
	return cleanDir
}

func ValidateNameStem(name string) error {
	if name == "" {
		return errors.New("文件名不能为空")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("文件名不能全为空白字符")
	}
	if strings.ContainsAny(name, windowsIllegalChars) {
		return fmt.Errorf("文件名包含 Windows 非法字符: %s", windowsIllegalChars)
	}
	if strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".") {
		return errors.New("文件名不能以空格或点结尾")
	}
	return nil
}

func ValidateRenameTarget(entry FileEntry, desiredNameStem string, occupiedPaths map[string]string) *ConflictResult {
	if err := ValidateNameStem(desiredNameStem); err != nil {
		return &ConflictResult{
			RecordID:     entry.RecordID,
			ConflictType: "invalid_name",
			TargetPath:   BuildRelativePath(entry.RelativeDir, desiredNameStem, entry.Ext),
			Reason:       err.Error(),
			Suggestion:   "请修改为合法且非空的名称",
		}
	}

	targetPath := strings.ToLower(BuildRelativePath(entry.RelativeDir, desiredNameStem, entry.Ext))
	currentPath := strings.ToLower(entry.LogicalPath())
	if targetPath == currentPath {
		return nil
	}
	if owner, exists := occupiedPaths[targetPath]; exists && owner != entry.RecordID {
		return &ConflictResult{
			RecordID:     entry.RecordID,
			ConflictType: "target_exists",
			TargetPath:   BuildRelativePath(entry.RelativeDir, desiredNameStem, entry.Ext),
			Reason:       "目标文件名在同目录下已存在",
			Suggestion:   "请改用其他文件名，避免覆盖现有文件",
		}
	}
	return nil
}

func DetectWorkbookDiff(previousEntries map[string]FileEntry, rows []WorkbookRow) WorkbookDiff {
	diff := WorkbookDiff{
		RenameRequests: make([]RenameRequest, 0),
		Conflicts:      make([]ConflictResult, 0),
		MissingRows:    make([]WorkbookRow, 0),
	}

	occupied := make(map[string]string, len(previousEntries))
	for recordID, entry := range previousEntries {
		occupied[strings.ToLower(entry.LogicalPath())] = recordID
	}

	for _, row := range rows {
		if row.RecordID == "" || row.Ext == "" {
			diff.Conflicts = append(diff.Conflicts, ConflictResult{
				RecordID:     row.RecordID,
				ConflictType: "metadata_corrupted",
				TargetPath:   BuildRelativePath(row.RelativeDir, row.NameStem, row.Ext),
				Reason:       "工作簿隐藏列或只读列已损坏",
				Suggestion:   "请刷新工作簿以恢复元数据",
			})
			continue
		}

		entry, exists := previousEntries[row.RecordID]
		if !exists {
			diff.MissingRows = append(diff.MissingRows, row)
			diff.Conflicts = append(diff.Conflicts, ConflictResult{
				RecordID:     row.RecordID,
				ConflictType: "missing_snapshot",
				TargetPath:   BuildRelativePath(row.RelativeDir, row.NameStem, row.Ext),
				Reason:       "快照中不存在该记录，无法安全同步",
				Suggestion:   "请执行一次全量刷新以重建快照",
			})
			continue
		}

		if row.NameStem == entry.NameStem {
			continue
		}

		if conflict := ValidateRenameTarget(entry, row.NameStem, occupied); conflict != nil {
			diff.Conflicts = append(diff.Conflicts, *conflict)
			continue
		}

		targetPath := entry.AbsolutePath
		if entry.AbsolutePath != "" {
			targetPath = filepath.Join(filepath.Dir(entry.AbsolutePath), row.NameStem+entry.Ext)
		}

		diff.RenameRequests = append(diff.RenameRequests, RenameRequest{
			RecordID:   entry.RecordID,
			OldPath:    entry.AbsolutePath,
			TargetPath: targetPath,
			Entry:      entry,
			Row:        row,
		})
	}

	sort.Slice(diff.RenameRequests, func(i, j int) bool {
		return diff.RenameRequests[i].OldPath < diff.RenameRequests[j].OldPath
	})

	return diff
}

func SnapshotRows(entries map[string]FileEntry, snapshotVersion int64) []WorkbookRow {
	if len(entries) == 0 {
		return []WorkbookRow{}
	}

	sortedEntries := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		sortedEntries = append(sortedEntries, entry)
	}

	sort.Slice(sortedEntries, func(i, j int) bool {
		return strings.ToLower(sortedEntries[i].LogicalPath()) < strings.ToLower(sortedEntries[j].LogicalPath())
	})

	rows := make([]WorkbookRow, 0, len(sortedEntries))
	for index, entry := range sortedEntries {
		rows = append(rows, WorkbookRow{
			RowIndex:      index + 2,
			RecordID:      entry.RecordID,
			NameStem:      entry.NameStem,
			Ext:           entry.Ext,
			RelativeDir:   entry.RelativeDir,
			Status:        entry.CurrentStatus(),
			Fingerprint:   entry.Fingerprint,
			LastKnownPath: entry.LogicalPath(),
			RowVersion:    snapshotVersion,
			Tombstone:     !entry.Exists,
			LastOpSource:  string(EventSourceInternal),
		})
	}

	return rows
}
