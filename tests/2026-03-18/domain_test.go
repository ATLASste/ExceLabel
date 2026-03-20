package domain_test

import (
	"testing"

	"excelabel/internal/domain"
)

func TestValidateNameStem(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{name: "合同清单", wantErr: false},
		{name: "", wantErr: true},
		{name: "   ", wantErr: true},
		{name: "报价?单", wantErr: true},
		{name: "尾部空格 ", wantErr: true},
		{name: "尾部点.", wantErr: true},
	}

	for _, item := range cases {
		err := domain.ValidateNameStem(item.name)
		if item.wantErr && err == nil {
			t.Fatalf("expected error for %q", item.name)
		}
		if !item.wantErr && err != nil {
			t.Fatalf("unexpected error for %q: %v", item.name, err)
		}
	}
}

func TestDetectWorkbookDiff(t *testing.T) {
	entries := map[string]domain.FileEntry{
		"1": {
			RecordID:     "1",
			NameStem:     "旧名称",
			Ext:          ".txt",
			RelativeDir:  "docs",
			AbsolutePath: `D:\\demo\\docs\\旧名称.txt`,
			Exists:       true,
		},
	}

	rows := []domain.WorkbookRow{
		{
			RowIndex:    2,
			RecordID:    "1",
			NameStem:    "新名称",
			Ext:         ".txt",
			RelativeDir: "docs",
		},
	}

	diff := domain.DetectWorkbookDiff(entries, rows)
	if len(diff.RenameRequests) != 1 {
		t.Fatalf("expected 1 rename request, got %d", len(diff.RenameRequests))
	}
	if len(diff.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(diff.Conflicts))
	}
}
