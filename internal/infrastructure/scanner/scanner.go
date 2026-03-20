package scanner

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"excelabel/internal/domain"
)

type Scanner struct {
	MaxWorkers int
}

func New(maxWorkers int) *Scanner {
	if maxWorkers <= 0 {
		maxWorkers = runtime.NumCPU()
		if maxWorkers < 2 {
			maxWorkers = 2
		}
	}

	return &Scanner{MaxWorkers: maxWorkers}
}

func (scanner *Scanner) ScanFull(rootDir string, snapshotVersion int64) ([]domain.FileEntry, error) {
	rootDir = filepath.Clean(rootDir)

	paths := make(chan string, scanner.MaxWorkers*2)
	results := make(chan domain.FileEntry, scanner.MaxWorkers*2)
	errs := make(chan error, 1)
	var workers sync.WaitGroup

	for index := 0; index < scanner.MaxWorkers; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for path := range paths {
				entry, err := buildEntry(rootDir, path, snapshotVersion)
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					continue
				}
				results <- entry
			}
		}()
	}

	walkDone := make(chan struct{})
	go func() {
		defer close(walkDone)
		defer close(paths)
		_ = filepath.WalkDir(rootDir, func(path string, dirEntry os.DirEntry, err error) error {
			if err != nil {
				select {
				case errs <- err:
				default:
				}
				return nil
			}
			if dirEntry.IsDir() {
				return nil
			}
			if shouldIgnoreFile(dirEntry.Name()) {
				return nil
			}
			paths <- path
			return nil
		})
	}()

	go func() {
		workers.Wait()
		close(results)
	}()

	entries := make([]domain.FileEntry, 0, 256)
	for entry := range results {
		entries = append(entries, entry)
	}

	select {
	case err := <-errs:
		return entries, err
	default:
	}

	<-walkDone
	return entries, nil
}

func buildEntry(rootDir, absolutePath string, snapshotVersion int64) (domain.FileEntry, error) {
	info, err := os.Stat(absolutePath)
	if err != nil {
		return domain.FileEntry{}, fmt.Errorf("stat file %s: %w", absolutePath, err)
	}

	relPath, err := filepath.Rel(rootDir, absolutePath)
	if err != nil {
		return domain.FileEntry{}, fmt.Errorf("build relative path %s: %w", absolutePath, err)
	}

	relDir := domain.NormalizeRelativeDir(filepath.Dir(relPath))
	fileName := filepath.Base(absolutePath)
	ext := filepath.Ext(fileName)
	nameStem := strings.TrimSuffix(fileName, ext)
	fingerprint, err := CalculateFingerprint(absolutePath, info)
	if err != nil {
		return domain.FileEntry{}, err
	}

	return domain.FileEntry{
		RecordID:        uuid.NewString(),
		NameStem:        nameStem,
		Ext:             ext,
		RelativeDir:     relDir,
		AbsolutePath:    absolutePath,
		Fingerprint:     fingerprint,
		Size:            info.Size(),
		ModTime:         info.ModTime(),
		Exists:          true,
		LastSeenVersion: snapshotVersion,
	}, nil
}

func CalculateFingerprint(absolutePath string, info os.FileInfo) (string, error) {
	file, err := os.Open(absolutePath)
	if err != nil {
		return "", fmt.Errorf("open file %s: %w", absolutePath, err)
	}
	defer file.Close()

	hash := sha1.New()
	if _, err := io.CopyN(hash, file, 4096); err != nil && err != io.EOF {
		return "", fmt.Errorf("read fingerprint bytes %s: %w", absolutePath, err)
	}

	meta := fmt.Sprintf("%d|%d", info.Size(), info.ModTime().UTC().UnixNano())
	_, _ = hash.Write([]byte(meta))
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func shouldIgnoreFile(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "~$") {
		return true
	}
	return false
}

func BuildState(entries []domain.FileEntry, rootDir, workbookPath string, snapshotVersion int64) domain.WorkspaceState {
	entryMap := make(map[string]domain.FileEntry, len(entries))
	for _, entry := range entries {
		entryMap[entry.RecordID] = entry
	}

	return domain.WorkspaceState{
		RootDir:         rootDir,
		WorkbookPath:    workbookPath,
		SnapshotVersion: snapshotVersion,
		LastScanAt:      time.Now(),
		Entries:         entryMap,
		PendingEvents:   make([]domain.SyncEvent, 0),
	}
}
