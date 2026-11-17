package domain

import (
	"strings"
	"testing"
)

func TestSnapshotRowsKeepsDirectoryAndExtensionGroupsContiguous(t *testing.T) {
	entries := map[string]FileEntry{
		"one":   {RecordID: "one", RelativeDir: "project", NameStem: "a", Ext: ".txt"},
		"two":   {RecordID: "two", RelativeDir: "project", NameStem: "b", Ext: ".pdf"},
		"three": {RecordID: "three", RelativeDir: "PROJECT", NameStem: "c", Ext: ".TXT"},
		"four":  {RecordID: "four", RelativeDir: "project", NameStem: "d", Ext: ".csv"},
		"five":  {RecordID: "five", RelativeDir: "project", NameStem: "e", Ext: ".txt"},
		"six":   {RecordID: "six", RelativeDir: "project/sub", NameStem: "f", Ext: ".txt"},
		"seven": {RecordID: "seven", RelativeDir: "", NameStem: "g", Ext: ".txt"},
	}

	rows := SnapshotRows(entries, 123)
	if len(rows) != len(entries) {
		t.Fatalf("got %d rows, want %d", len(rows), len(entries))
	}
	seenGroups := make(map[string]bool)
	previousGroup := ""
	for index, row := range rows {
		if row.RowIndex != index+2 {
			t.Errorf("row index = %d, want %d", row.RowIndex, index+2)
		}
		entry := entries[row.RecordID]
		if row.LastKnownPath != entry.LogicalPath() || row.RowVersion != 123 {
			t.Errorf("row metadata no longer matches entry: %+v", row)
		}
		group := strings.ToLower(NormalizeRelativeDir(row.RelativeDir)) + "\x00" + strings.ToLower(row.Ext)
		if group != previousGroup {
			if seenGroups[group] {
				t.Fatalf("directory/extension group %q is split by another group", group)
			}
			seenGroups[group] = true
			previousGroup = group
		}
	}
}
