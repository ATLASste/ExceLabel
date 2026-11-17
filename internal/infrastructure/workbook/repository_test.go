package workbook

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"excelabel/internal/domain"
)

func TestWorkbookColumnsAndStatus(t *testing.T) {
	path, repository := createTestWorkbook(t)
	file := openTestWorkbook(t, path)

	expected := map[string]string{
		"A1": "原文件名",
		"B1": "NameStem (在此修改)",
		"C1": "Ext",
		"D1": "RelativeDir",
		"E1": "Status",
		"A2": "原始名称",
		"B2": "原始名称",
		"C2": ".txt",
		"D2": "folder",
		"E2": "pending",
	}
	for axis, want := range expected {
		if got := testCellValue(t, file, axis); got != want {
			t.Errorf("%s = %q, want %q", axis, got, want)
		}
	}

	styleID, err := file.GetCellStyle(filesSheet, "B1")
	if err != nil {
		t.Fatal(err)
	}
	style, err := file.GetStyle(styleID)
	if err != nil {
		t.Fatal(err)
	}
	if len(style.Fill.Color) != 1 || strings.TrimPrefix(strings.ToUpper(style.Fill.Color[0]), "FF") != "F97316" {
		t.Errorf("B1 fill = %v, want orange", style.Fill.Color)
	}
	visible, err := file.GetSheetVisible(metaSheet)
	if err != nil {
		t.Fatal(err)
	}
	if visible {
		t.Error("metadata sheet should be hidden")
	}

	if err := file.SetCellValue(filesSheet, "B2", "修改后"); err != nil {
		t.Fatal(err)
	}
	if err := file.Save(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	rows, err := repository.LoadRows(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].NameStem != "修改后" || rows[0].Ext != ".txt" ||
		rows[0].RelativeDir != "folder" || rows[0].RecordID != "record-1" {
		t.Fatalf("unexpected loaded rows: %+v", rows)
	}
	if err := repository.WriteStatuses(path, []domain.WorkbookRow{{RowIndex: 2, Status: "updated"}}); err != nil {
		t.Fatal(err)
	}
	file = openTestWorkbook(t, path)
	for axis, want := range map[string]string{"A2": "原始名称", "B2": "修改后", "D2": "folder", "E2": "updated"} {
		if got := testCellValue(t, file, axis); got != want {
			t.Errorf("%s after status update = %q, want %q", axis, got, want)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	var sheet struct {
		AutoFilter struct {
			Ref string `xml:"ref,attr"`
		} `xml:"autoFilter"`
	}
	if err := xml.Unmarshal(readSheetXML(t, path), &sheet); err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(sheet.AutoFilter.Ref, "$", "") != "A1:E2" {
		t.Errorf("filter range = %q, want A1:E2", sheet.AutoFilter.Ref)
	}
}

func TestLegacyWorkbookColumns(t *testing.T) {
	path, repository := createTestWorkbook(t)
	file := openTestWorkbook(t, path)
	makeLegacyWorkbook(t, file)
	if err := file.SetCellValue(filesSheet, "A2", "legacy edited"); err != nil {
		t.Fatal(err)
	}
	if err := file.Save(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	rows, err := repository.LoadRows(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].NameStem != "legacy edited" || rows[0].Ext != ".txt" || rows[0].RelativeDir != "folder" {
		t.Fatalf("unexpected legacy rows: %+v", rows)
	}
	if err := repository.WriteStatuses(path, []domain.WorkbookRow{{RowIndex: 2, Status: "legacy status"}}); err != nil {
		t.Fatal(err)
	}
	file = openTestWorkbook(t, path)
	defer func() { _ = file.Close() }()
	if got := testCellValue(t, file, "C2"); got != "folder" {
		t.Errorf("legacy directory = %q, want folder", got)
	}
	if got := testCellValue(t, file, "D2"); got != "legacy status" {
		t.Errorf("legacy status = %q, want legacy status", got)
	}
}

func TestRejectsChangedHeadersAndExcelErrors(t *testing.T) {
	t.Run("changed header", func(t *testing.T) {
		path, repository := createTestWorkbook(t)
		file := openTestWorkbook(t, path)
		if err := file.SetCellValue(filesSheet, "C1", "unexpected"); err != nil {
			t.Fatal(err)
		}
		if err := file.Save(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.LoadRows(path); err == nil || !strings.Contains(err.Error(), "C1") {
			t.Fatalf("LoadRows error = %v, want a C1 header error", err)
		}
		if err := repository.WriteStatuses(path, nil); err == nil || !strings.Contains(err.Error(), "C1") {
			t.Fatalf("WriteStatuses error = %v, want a C1 header error", err)
		}
	})

	for _, legacy := range []bool{false, true} {
		name := "current"
		axis := "B2"
		if legacy {
			name, axis = "legacy", "A2"
		}
		t.Run(name+" formula error", func(t *testing.T) {
			path, repository := createTestWorkbook(t)
			file := openTestWorkbook(t, path)
			if legacy {
				makeLegacyWorkbook(t, file)
			}
			if err := file.SetCellValue(filesSheet, axis, "#VALUE!"); err != nil {
				t.Fatal(err)
			}
			if err := file.Save(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			rows, err := repository.LoadRows(path)
			if err != nil || len(rows) != 1 || rows[0].NameStem != "#VALUE!" {
				t.Fatalf("literal error-like name was rejected: rows=%+v err=%v", rows, err)
			}

			replaceSheetCell(t, path, axis, `<c r="`+axis+`" t="e"><f>VALUE("bad")</f><v>#VALUE!</v></c>`)
			if _, err := repository.LoadRows(path); err == nil || !strings.Contains(err.Error(), axis) || !strings.Contains(err.Error(), "#VALUE!") {
				t.Fatalf("LoadRows error = %v, want an Excel error at %s", err, axis)
			}
		})
	}
}

func createTestWorkbook(t *testing.T) (string, *Repository) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "files.xlsx")
	repository := New()
	rows := []domain.WorkbookRow{{
		RowIndex:      2,
		RecordID:      "record-1",
		NameStem:      "原始名称",
		Ext:           ".txt",
		RelativeDir:   "folder",
		Status:        "pending",
		LastKnownPath: "folder/原始名称.txt",
	}}
	if err := repository.CreateWorkbook(path, rows, domain.WorkspaceState{}); err != nil {
		t.Fatal(err)
	}
	return path, repository
}

func openTestWorkbook(t *testing.T, path string) *excelize.File {
	t.Helper()
	file, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func testCellValue(t *testing.T, file *excelize.File, axis string) string {
	t.Helper()
	value, err := file.GetCellValue(filesSheet, axis)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func makeLegacyWorkbook(t *testing.T, file *excelize.File) {
	t.Helper()
	for row := 1; row <= 2; row++ {
		for column := 1; column <= 4; column++ {
			source, err := excelize.CoordinatesToCellName(column+1, row)
			if err != nil {
				t.Fatal(err)
			}
			target, err := excelize.CoordinatesToCellName(column, row)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.SetCellValue(filesSheet, target, testCellValue(t, file, source)); err != nil {
				t.Fatal(err)
			}
		}
		axis, err := excelize.CoordinatesToCellName(5, row)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.SetCellValue(filesSheet, axis, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.AutoFilter(filesSheet, "A1:D2", nil); err != nil {
		t.Fatal(err)
	}
}

func readSheetXML(t *testing.T, path string) []byte {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	for _, entry := range reader.File {
		if entry.Name == "xl/worksheets/sheet1.xml" {
			file, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			return data
		}
	}
	t.Fatal("worksheet XML not found")
	return nil
}

func replaceSheetCell(t *testing.T, path, axis, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	replaced := false
	for _, entry := range reader.File {
		file, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if entry.Name == "xl/worksheets/sheet1.xml" {
			start := bytes.Index(content, []byte(`<c r="`+axis+`"`))
			if start < 0 {
				t.Fatalf("cell %s not found in worksheet XML", axis)
			}
			end := bytes.Index(content[start:], []byte("</c>"))
			if end < 0 {
				t.Fatalf("cell %s has no closing tag", axis)
			}
			end += start + len("</c>")
			updated := append([]byte{}, content[:start]...)
			updated = append(updated, replacement...)
			content = append(updated, content[end:]...)
			replaced = true
		}
		header := entry.FileHeader
		output, err := writer.CreateHeader(&header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := output.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if !replaced {
		t.Fatal("worksheet XML not found")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, result.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
