package workbook

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"excelabel/internal/domain"
)

const (
	filesSheet = "Files"
	metaSheet  = "_meta"
)

type Repository struct{}

func New() *Repository {
	return &Repository{}
}

func (repository *Repository) CreateWorkbook(workbookPath string, rows []domain.WorkbookRow, state domain.WorkspaceState) error {
	file := excelize.NewFile()
	defaultSheet := file.GetSheetName(0)
	if defaultSheet != filesSheet {
		file.SetSheetName(defaultSheet, filesSheet)
	}
	if _, err := file.NewSheet(metaSheet); err != nil {
		return fmt.Errorf("create meta sheet: %w", err)
	}

	if err := writeHeaders(file); err != nil {
		return err
	}
	if err := writeRows(file, rows); err != nil {
		return err
	}
	if err := writeMeta(file, state, rows); err != nil {
		return err
	}
	if err := applyLayout(file, rows); err != nil {
		return err
	}
	filesSheetIndex, err := file.GetSheetIndex(filesSheet)
	if err != nil {
		return fmt.Errorf("resolve active sheet: %w", err)
	}
	file.SetActiveSheet(filesSheetIndex)
	if err := file.SaveAs(workbookPath); err != nil {
		return fmt.Errorf("请先关闭对应工作簿；save workbook: %w", err)
	}
	return nil
}

func (repository *Repository) LoadRows(workbookPath string) ([]domain.WorkbookRow, error) {
	file, err := excelize.OpenFile(workbookPath)
	if err != nil {
		return nil, fmt.Errorf("open workbook: %w", err)
	}
	defer func() { _ = file.Close() }()

	rows, err := file.GetRows(filesSheet)
	if err != nil {
		return nil, fmt.Errorf("read files sheet rows: %w", err)
	}
	if len(rows) <= 1 {
		return []domain.WorkbookRow{}, nil
	}

	metadataByRow, err := loadMetadataRows(file)
	if err != nil {
		return nil, err
	}

	result := make([]domain.WorkbookRow, 0, len(rows)-1)
	for index := 1; index < len(rows); index++ {
		rowIndex := index + 1
		row := rows[index]
		metadata := metadataByRow[rowIndex]
		result = append(result, domain.WorkbookRow{
			RowIndex:      rowIndex,
			NameStem:      cell(row, 0),
			Ext:           cell(row, 1),
			RelativeDir:   cell(row, 2),
			Status:        cell(row, 3),
			RecordID:      metadata.RecordID,
			Fingerprint:   metadata.Fingerprint,
			LastKnownPath: metadata.LastKnownPath,
			RowVersion:    metadata.RowVersion,
			Tombstone:     metadata.Tombstone,
			LastOpSource:  metadata.LastOpSource,
		})
	}

	return result, nil
}

func (repository *Repository) WriteStatuses(workbookPath string, rows []domain.WorkbookRow) error {
	file, err := excelize.OpenFile(workbookPath)
	if err != nil {
		return fmt.Errorf("open workbook for statuses: %w", err)
	}
	defer func() { _ = file.Close() }()

	styles, err := buildStyles(file)
	if err != nil {
		return err
	}
	metadataByRow, err := loadMetadataRows(file)
	if err != nil {
		return err
	}

	for _, row := range rows {
		axis := fmt.Sprintf("D%d", row.RowIndex)
		if err := file.SetCellValue(filesSheet, axis, row.Status); err != nil {
			return fmt.Errorf("write status %s: %w", axis, err)
		}
		if err := applyStatusCellStyle(file, axis, row.Status, styles); err != nil {
			return fmt.Errorf("apply status style %s: %w", axis, err)
		}

		metadata := metadataByRow[row.RowIndex]
		if metadata.RowIndex == 0 {
			metadata.RowIndex = row.RowIndex
		}
		if row.RecordID != "" {
			metadata.RecordID = row.RecordID
		}
		if row.Fingerprint != "" {
			metadata.Fingerprint = row.Fingerprint
		}
		if row.LastKnownPath != "" {
			metadata.LastKnownPath = row.LastKnownPath
		}
		if row.RowVersion != 0 {
			metadata.RowVersion = row.RowVersion
		}
		if row.Tombstone {
			metadata.Tombstone = true
		}
		if row.LastOpSource != "" {
			metadata.LastOpSource = row.LastOpSource
		}
		if metadata.RecordID != "" {
			if err := writeMetadataRow(file, metadata); err != nil {
				return err
			}
		}
	}

	if err := file.Save(); err != nil {
		return fmt.Errorf("请先关闭对应工作簿；save workbook statuses: %w", err)
	}
	return nil
}

func (repository *Repository) UpdateRows(workbookPath string, rows []domain.WorkbookRow, state domain.WorkspaceState) error {
	return repository.CreateWorkbook(workbookPath, rows, state)
}

func writeHeaders(file *excelize.File) error {
	headers := []string{"NameStem (在此修改)", "Ext", "RelativeDir", "Status"}
	for index, header := range headers {
		axis, err := excelize.CoordinatesToCellName(index+1, 1)
		if err != nil {
			return fmt.Errorf("build header axis: %w", err)
		}
		if err := file.SetCellValue(filesSheet, axis, header); err != nil {
			return fmt.Errorf("write header %s: %w", axis, err)
		}
	}
	return nil
}

func writeRows(file *excelize.File, rows []domain.WorkbookRow) error {
	for _, row := range rows {
		values := []any{row.NameStem, row.Ext, row.RelativeDir, row.Status}
		for index, value := range values {
			axis, err := excelize.CoordinatesToCellName(index+1, row.RowIndex)
			if err != nil {
				return fmt.Errorf("build row axis: %w", err)
			}
			if err := file.SetCellValue(filesSheet, axis, value); err != nil {
				return fmt.Errorf("write row cell %s: %w", axis, err)
			}
		}
	}
	return nil
}

func writeMeta(file *excelize.File, state domain.WorkspaceState, rows []domain.WorkbookRow) error {
	metaEntries := []struct {
		key   string
		value any
	}{
		{key: "RootDir", value: state.RootDir},
		{key: "WorkbookPath", value: state.WorkbookPath},
		{key: "SnapshotVersion", value: state.SnapshotVersion},
		{key: "LastScanAt", value: state.LastScanAt.Format(time.RFC3339)},
		{key: "LastWorkbookHash", value: state.LastWorkbookHash},
	}

	for rowIndex, entry := range metaEntries {
		excelRow := rowIndex + 1
		if err := file.SetCellValue(metaSheet, fmt.Sprintf("A%d", excelRow), entry.key); err != nil {
			return fmt.Errorf("write meta key: %w", err)
		}
		if err := file.SetCellValue(metaSheet, fmt.Sprintf("B%d", excelRow), entry.value); err != nil {
			return fmt.Errorf("write meta value: %w", err)
		}
	}

	metadataHeaders := []string{"RowIndex", "RecordID", "FileFingerprint", "LastKnownPath", "RowVersion", "Tombstone", "LastOpSource"}
	for index, header := range metadataHeaders {
		axis, err := excelize.CoordinatesToCellName(index+4, 1)
		if err != nil {
			return fmt.Errorf("build metadata header axis: %w", err)
		}
		if err := file.SetCellValue(metaSheet, axis, header); err != nil {
			return fmt.Errorf("write metadata header %s: %w", axis, err)
		}
	}

	for _, row := range rows {
		if err := writeMetadataRow(file, row); err != nil {
			return err
		}
	}
	return nil
}

func writeMetadataRow(file *excelize.File, row domain.WorkbookRow) error {
	values := []any{row.RowIndex, row.RecordID, row.Fingerprint, row.LastKnownPath, row.RowVersion, row.Tombstone, row.LastOpSource}
	for index, value := range values {
		axis, err := excelize.CoordinatesToCellName(index+4, row.RowIndex)
		if err != nil {
			return fmt.Errorf("build metadata axis: %w", err)
		}
		if err := file.SetCellValue(metaSheet, axis, value); err != nil {
			return fmt.Errorf("write metadata cell %s: %w", axis, err)
		}
	}
	return nil
}

func loadMetadataRows(file *excelize.File) (map[int]domain.WorkbookRow, error) {
	rows, err := file.GetRows(metaSheet)
	if err != nil {
		return nil, fmt.Errorf("read meta sheet rows: %w", err)
	}

	metadataByRow := make(map[int]domain.WorkbookRow)
	for index := 1; index < len(rows); index++ {
		row := rows[index]
		rowIndex := parseInt(cell(row, 3))
		if rowIndex == 0 {
			continue
		}
		metadataByRow[rowIndex] = domain.WorkbookRow{
			RowIndex:      rowIndex,
			RecordID:      cell(row, 4),
			Fingerprint:   cell(row, 5),
			LastKnownPath: cell(row, 6),
			RowVersion:    parseInt64(cell(row, 7)),
			Tombstone:     strings.EqualFold(cell(row, 8), "true"),
			LastOpSource:  cell(row, 9),
		}
	}
	return metadataByRow, nil
}

func applyLayout(file *excelize.File, rows []domain.WorkbookRow) error {
	styles, err := buildStyles(file)
	if err != nil {
		return err
	}
	if err := applyVisibleColumnWidths(file, rows); err != nil {
		return err
	}
	if err := file.SetSheetVisible(metaSheet, false); err != nil {
		return fmt.Errorf("hide meta sheet: %w", err)
	}
	if err := file.SetCellStyle(filesSheet, "A1", "A1", styles.headerPrimaryStyle); err != nil {
		return fmt.Errorf("apply primary header style: %w", err)
	}
	if err := file.SetCellStyle(filesSheet, "B1", "D1", styles.headerStyle); err != nil {
		return fmt.Errorf("apply header style: %w", err)
	}

	dataEndRow := len(rows) + 1
	if dataEndRow < 2 {
		dataEndRow = 2
	}
	if err := file.SetPanes(filesSheet, &excelize.Panes{Freeze: true, Split: false, XSplit: 0, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return fmt.Errorf("freeze header row: %w", err)
	}
	filterRange := fmt.Sprintf("A1:D%d", dataEndRow)
	if err := file.AutoFilter(filesSheet, filterRange, []excelize.AutoFilterOptions{}); err != nil {
		return fmt.Errorf("set auto filter: %w", err)
	}
	for _, row := range rows {
		axis := fmt.Sprintf("D%d", row.RowIndex)
		if err := applyStatusCellStyle(file, axis, row.Status, styles); err != nil {
			return fmt.Errorf("apply initial status style %s: %w", axis, err)
		}
	}
	return nil
}

type workbookStyles struct {
	headerStyle        int
	headerPrimaryStyle int
	statusDefaultStyle int
	statusSyncedStyle  int
}

func buildStyles(file *excelize.File) (workbookStyles, error) {
	headerStyle, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 12, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#16A34A"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return workbookStyles{}, fmt.Errorf("build header style: %w", err)
	}
	headerPrimaryStyle, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 12, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#F97316"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return workbookStyles{}, fmt.Errorf("build primary header style: %w", err)
	}
	statusDefaultStyle, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Color: "#1F2937"},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
	})
	if err != nil {
		return workbookStyles{}, fmt.Errorf("build default status style: %w", err)
	}
	statusSyncedStyle, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#16A34A"},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
	})
	if err != nil {
		return workbookStyles{}, fmt.Errorf("build synced status style: %w", err)
	}
	return workbookStyles{
		headerStyle:        headerStyle,
		headerPrimaryStyle: headerPrimaryStyle,
		statusDefaultStyle: statusDefaultStyle,
		statusSyncedStyle:  statusSyncedStyle,
	}, nil
}

func applyVisibleColumnWidths(file *excelize.File, rows []domain.WorkbookRow) error {
	widths := map[string]float64{
		"A": estimateColumnWidth("NameStem (在此修改)"),
		"B": estimateColumnWidth("Ext"),
		"C": estimateColumnWidth("RelativeDir"),
		"D": estimateColumnWidth("Status"),
	}
	for _, row := range rows {
		widths["A"] = maxWidth(widths["A"], estimateColumnWidth(row.NameStem))
		widths["B"] = maxWidth(widths["B"], estimateColumnWidth(row.Ext))
		widths["C"] = maxWidth(widths["C"], estimateColumnWidth(row.RelativeDir))
		widths["D"] = maxWidth(widths["D"], estimateColumnWidth(row.Status))
	}
	for _, column := range []string{"A", "B", "C", "D"} {
		if err := file.SetColWidth(filesSheet, column, column, widths[column]); err != nil {
			return fmt.Errorf("set width for column %s: %w", column, err)
		}
	}
	return nil
}

func applyStatusCellStyle(file *excelize.File, axis, status string, styles workbookStyles) error {
	styleID := styles.statusDefaultStyle
	if strings.HasPrefix(status, string(domain.StatusSynced)) {
		styleID = styles.statusSyncedStyle
	}
	if err := file.SetCellStyle(filesSheet, axis, axis, styleID); err != nil {
		return err
	}
	return nil
}

func estimateColumnWidth(value string) float64 {
	trimmed := strings.TrimSpace(value)
	width := float64(len([]rune(trimmed)) + 4)
	if width < 12 {
		return 12
	}
	if width > 48 {
		return 48
	}
	return width
}

func maxWidth(current, next float64) float64 {
	if next > current {
		return next
	}
	return current
}

func cell(row []string, index int) string {
	if index >= len(row) {
		return ""
	}
	return row[index]
}

func parseInt(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func parseInt64(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
