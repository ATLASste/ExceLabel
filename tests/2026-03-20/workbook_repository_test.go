package workbook_test

import (
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"excelabel/internal/domain"
	"excelabel/internal/infrastructure/workbook"
)

func TestCreateWorkbookStoresMetadataOutsideVisibleSheet(t *testing.T) {
	t.Parallel()

	repository := workbook.New()
	workbookPath := t.TempDir() + "/excelabel.xlsx"
	rows := []domain.WorkbookRow{
		{
			RowIndex:      2,
			RecordID:      "id-1",
			NameStem:      "合同清单",
			Ext:           ".xlsx",
			RelativeDir:   "docs",
			Status:        string(domain.StatusSynced),
			Fingerprint:   "fp-1",
			LastKnownPath: "docs/合同清单.xlsx",
			RowVersion:    101,
			Tombstone:     false,
			LastOpSource:  string(domain.EventSourceInternal),
		},
	}
	state := domain.WorkspaceState{
		RootDir:          `D:\\demo`,
		WorkbookPath:     workbookPath,
		SnapshotVersion:  101,
		LastScanAt:       time.Date(2026, 3, 20, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)),
		LastWorkbookHash: "hash-1",
	}

	if err := repository.CreateWorkbook(workbookPath, rows, state); err != nil {
		t.Fatalf("CreateWorkbook() error = %v", err)
	}

	file, err := excelize.OpenFile(workbookPath)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	defer func() { _ = file.Close() }()

	visibleValue, err := file.GetCellValue("Files", "E2")
	if err != nil {
		t.Fatalf("GetCellValue(Files!E2) error = %v", err)
	}
	if visibleValue != "" {
		t.Fatalf("expected Files!E2 to be empty, got %q", visibleValue)
	}

	metaRecordID, err := file.GetCellValue("_meta", "E2")
	if err != nil {
		t.Fatalf("GetCellValue(_meta!E2) error = %v", err)
	}
	if metaRecordID != "id-1" {
		t.Fatalf("expected _meta!E2 to store record id, got %q", metaRecordID)
	}

	loadedRows, err := repository.LoadRows(workbookPath)
	if err != nil {
		t.Fatalf("LoadRows() error = %v", err)
	}
	if len(loadedRows) != 1 {
		t.Fatalf("expected 1 loaded row, got %d", len(loadedRows))
	}
	if loadedRows[0].RecordID != "id-1" {
		t.Fatalf("expected loaded row record id to be restored from _meta, got %q", loadedRows[0].RecordID)
	}
	if loadedRows[0].Fingerprint != "fp-1" {
		t.Fatalf("expected loaded row fingerprint to be restored from _meta, got %q", loadedRows[0].Fingerprint)
	}
}

func TestWriteStatusesUpdatesMetaSheetByRowIndex(t *testing.T) {
	t.Parallel()

	repository := workbook.New()
	workbookPath := t.TempDir() + "/excelabel.xlsx"
	rows := []domain.WorkbookRow{
		{
			RowIndex:      2,
			RecordID:      "id-1",
			NameStem:      "旧名称",
			Ext:           ".txt",
			RelativeDir:   "docs",
			Status:        string(domain.StatusPendingExcelChange),
			Fingerprint:   "fp-1",
			LastKnownPath: "docs/旧名称.txt",
			RowVersion:    101,
			Tombstone:     false,
			LastOpSource:  string(domain.EventSourceInternal),
		},
	}
	state := domain.WorkspaceState{
		RootDir:         `D:\\demo`,
		WorkbookPath:    workbookPath,
		SnapshotVersion: 101,
		LastScanAt:      time.Date(2026, 3, 20, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)),
	}

	if err := repository.CreateWorkbook(workbookPath, rows, state); err != nil {
		t.Fatalf("CreateWorkbook() error = %v", err)
	}

	statusUpdates := []domain.WorkbookRow{
		{
			RowIndex:      2,
			RecordID:      "id-1",
			Status:        string(domain.StatusSynced),
			LastKnownPath: "docs/新名称.txt",
			RowVersion:    202,
			LastOpSource:  string(domain.EventSourceWorkbook),
		},
	}
	if err := repository.WriteStatuses(workbookPath, statusUpdates); err != nil {
		t.Fatalf("WriteStatuses() error = %v", err)
	}

	loadedRows, err := repository.LoadRows(workbookPath)
	if err != nil {
		t.Fatalf("LoadRows() error = %v", err)
	}
	if len(loadedRows) != 1 {
		t.Fatalf("expected 1 loaded row, got %d", len(loadedRows))
	}
	if loadedRows[0].Status != string(domain.StatusSynced) {
		t.Fatalf("expected status to be updated, got %q", loadedRows[0].Status)
	}
	if loadedRows[0].LastKnownPath != "docs/新名称.txt" {
		t.Fatalf("expected last known path to be updated in _meta, got %q", loadedRows[0].LastKnownPath)
	}
	if loadedRows[0].RowVersion != 202 {
		t.Fatalf("expected row version to be updated in _meta, got %d", loadedRows[0].RowVersion)
	}
	if loadedRows[0].LastOpSource != string(domain.EventSourceWorkbook) {
		t.Fatalf("expected last op source to be updated in _meta, got %q", loadedRows[0].LastOpSource)
	}
	if loadedRows[0].RecordID != "id-1" {
		t.Fatalf("expected record id to remain stable, got %q", loadedRows[0].RecordID)
	}
}
