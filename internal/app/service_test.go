package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestApplyWorkbookChangesForFilteredFormulaRows(t *testing.T) {
	root := t.TempDir()
	for _, relativePath := range []string{
		filepath.Join("target", "100.txt"),
		filepath.Join("target", "150.csv"),
		filepath.Join("target", "200.txt"),
		filepath.Join("target", "250.csv"),
		filepath.Join("target", "300.txt"),
		filepath.Join("other", "400.txt"),
	} {
		path := filepath.Join(root, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(relativePath), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	workbookPath := filepath.Join(t.TempDir(), "files.xlsx")
	service := NewSyncService(Config{RootDir: root, WorkbookPath: workbookPath})
	if err := service.StartWorkspace(); err != nil {
		t.Fatal(err)
	}
	rows, err := service.workbook.LoadRows(workbookPath)
	if err != nil {
		t.Fatal(err)
	}
	file, err := excelize.OpenFile(workbookPath)
	if err != nil {
		t.Fatal(err)
	}
	selected := 0
	previousRow := 0
	for _, row := range rows {
		if row.RelativeDir != "target" || row.Ext != ".txt" {
			if err := file.SetRowVisible("Files", row.RowIndex, false); err != nil {
				t.Fatal(err)
			}
			continue
		}
		selected++
		if previousRow != 0 && row.RowIndex != previousRow+1 {
			t.Fatalf("matching rows are not contiguous: %d and %d", previousRow, row.RowIndex)
		}
		previousRow = row.RowIndex
		name, err := strconv.Atoi(row.NameStem)
		if err != nil {
			t.Fatal(err)
		}
		axis := fmt.Sprintf("B%d", row.RowIndex)
		if err := file.SetCellInt("Files", axis, int64(name+1)); err != nil {
			t.Fatal(err)
		}
		if err := file.SetCellFormula("Files", axis, fmt.Sprintf("=A%d+1", row.RowIndex)); err != nil {
			t.Fatal(err)
		}
	}
	if selected != 3 {
		t.Fatalf("selected %d rows, want 3", selected)
	}
	if err := file.Save(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if err := service.ApplyWorkbookChanges(); err != nil {
		t.Fatal(err)
	}
	for _, relativePath := range []string{
		filepath.Join("target", "101.txt"),
		filepath.Join("target", "201.txt"),
		filepath.Join("target", "301.txt"),
		filepath.Join("target", "150.csv"),
		filepath.Join("target", "250.csv"),
		filepath.Join("other", "400.txt"),
	} {
		if _, err := os.Stat(filepath.Join(root, relativePath)); err != nil {
			t.Errorf("expected file %s: %v", relativePath, err)
		}
	}
	for _, name := range []string{"100.txt", "200.txt", "300.txt"} {
		if _, err := os.Stat(filepath.Join(root, "target", name)); !os.IsNotExist(err) {
			t.Errorf("original target %s still exists or could not be checked: %v", name, err)
		}
	}

	file, err = excelize.OpenFile(workbookPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	for _, row := range rows {
		if row.RelativeDir != "target" || row.Ext != ".txt" {
			visible, err := file.GetRowVisible("Files", row.RowIndex)
			if err != nil || visible {
				t.Errorf("row %d should remain hidden: visible=%v err=%v", row.RowIndex, visible, err)
			}
			continue
		}
		original, err := file.GetCellValue("Files", fmt.Sprintf("A%d", row.RowIndex))
		if err != nil || original != row.NameStem {
			t.Errorf("row %d original name = %q, err=%v", row.RowIndex, original, err)
		}
		formula, err := file.GetCellFormula("Files", fmt.Sprintf("B%d", row.RowIndex))
		if err != nil || !strings.Contains(formula, fmt.Sprintf("A%d", row.RowIndex)) {
			t.Errorf("row %d formula = %q, err=%v", row.RowIndex, formula, err)
		}
		status, err := file.GetCellValue("Files", fmt.Sprintf("E%d", row.RowIndex))
		if err != nil || !strings.HasPrefix(status, "✅ 同步成功") {
			t.Errorf("row %d status = %q, err=%v", row.RowIndex, status, err)
		}
	}
}
