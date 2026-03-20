package fswatch

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"

	"excelabel/internal/domain"
)

type Watcher struct {
	watcher            *fsnotify.Watcher
	workbookPath       string
	rootDir            string
	fsDebounce         time.Duration
	workbookDebounce   time.Duration
	events             chan domain.SyncEvent
	closeCh            chan struct{}
	mu                 sync.Mutex
	pendingFilesystem  map[string]domain.SyncEvent
	pendingWorkbookHit bool
	closed             bool
}

func New(rootDir, workbookPath string, fsDebounce, workbookDebounce time.Duration) (*Watcher, error) {
	underlying, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if fsDebounce <= 0 {
		fsDebounce = 800 * time.Millisecond
	}
	if workbookDebounce <= 0 {
		workbookDebounce = 500 * time.Millisecond
	}

	watcher := &Watcher{
		watcher:           underlying,
		workbookPath:      filepath.Clean(workbookPath),
		rootDir:           filepath.Clean(rootDir),
		fsDebounce:        fsDebounce,
		workbookDebounce:  workbookDebounce,
		events:            make(chan domain.SyncEvent, 256),
		closeCh:           make(chan struct{}),
		pendingFilesystem: make(map[string]domain.SyncEvent),
	}

	go watcher.loop()
	return watcher, nil
}

func (watcher *Watcher) WatchRoot(rootDir string) error {
	return filepath.WalkDir(rootDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !dirEntry.IsDir() {
			return nil
		}
		return watcher.watcher.Add(path)
	})
}

func (watcher *Watcher) WatchWorkbook(workbookPath string) error {
	return watcher.watcher.Add(filepath.Dir(workbookPath))
}

func (watcher *Watcher) Events() <-chan domain.SyncEvent {
	return watcher.events
}

func (watcher *Watcher) Close() error {
	watcher.mu.Lock()
	if watcher.closed {
		watcher.mu.Unlock()
		return nil
	}
	watcher.closed = true
	close(watcher.closeCh)
	watcher.mu.Unlock()

	return watcher.watcher.Close()
}

func (watcher *Watcher) loop() {
	var fsTimer *time.Timer
	var workbookTimer *time.Timer
	var fsTimerCh <-chan time.Time
	var workbookTimerCh <-chan time.Time

	for {
		select {
		case event, ok := <-watcher.watcher.Events:
			if !ok {
				close(watcher.events)
				return
			}

			cleanName := filepath.Clean(event.Name)
			if cleanName == watcher.workbookPath {
				watcher.mu.Lock()
				watcher.pendingWorkbookHit = true
				watcher.mu.Unlock()
				if workbookTimer != nil {
					workbookTimer.Stop()
				}
				workbookTimer = time.NewTimer(watcher.workbookDebounce)
				workbookTimerCh = workbookTimer.C
				continue
			}

			if event.Has(fsnotify.Create) {
				_ = watcher.tryAddDirectory(cleanName)
			}

			watcher.mu.Lock()
			watcher.pendingFilesystem[cleanName] = toSyncEvent(event)
			watcher.mu.Unlock()
			if fsTimer != nil {
				fsTimer.Stop()
			}
			fsTimer = time.NewTimer(watcher.fsDebounce)
			fsTimerCh = fsTimer.C

		case <-fsTimerCh:
			watcher.flushFilesystem()
			fsTimerCh = nil

		case <-workbookTimerCh:
			watcher.flushWorkbook()
			workbookTimerCh = nil

		case <-watcher.watcher.Errors:
			// 首版忽略底层错误细节，后续可接入日志层输出

		case <-watcher.closeCh:
			close(watcher.events)
			return
		}
	}
}

func (watcher *Watcher) flushFilesystem() {
	watcher.mu.Lock()
	pending := watcher.pendingFilesystem
	watcher.pendingFilesystem = make(map[string]domain.SyncEvent)
	watcher.mu.Unlock()

	for _, event := range pending {
		watcher.events <- event
	}
}

func (watcher *Watcher) flushWorkbook() {
	watcher.mu.Lock()
	pending := watcher.pendingWorkbookHit
	watcher.pendingWorkbookHit = false
	watcher.mu.Unlock()

	if !pending {
		return
	}

	watcher.events <- domain.SyncEvent{
		EventID:        uuid.NewString(),
		Source:         domain.EventSourceWorkbook,
		Type:           domain.EventTypeUpdated,
		NewPath:        watcher.workbookPath,
		OccurredAt:     time.Now(),
		CorrelationKey: "workbook-save",
	}
}

func (watcher *Watcher) tryAddDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return err
	}
	return watcher.watcher.Add(path)
}

func toSyncEvent(event fsnotify.Event) domain.SyncEvent {
	eventType := domain.EventTypeUnknown
	switch {
	case event.Has(fsnotify.Create):
		eventType = domain.EventTypeCreated
	case event.Has(fsnotify.Write):
		eventType = domain.EventTypeUpdated
	case event.Has(fsnotify.Remove):
		eventType = domain.EventTypeDeleted
	case event.Has(fsnotify.Rename):
		eventType = domain.EventTypeRenamed
	}

	return domain.SyncEvent{
		EventID:        uuid.NewString(),
		Source:         domain.EventSourceFilesystem,
		Type:           eventType,
		NewPath:        filepath.Clean(event.Name),
		OccurredAt:     time.Now(),
		CorrelationKey: filepath.Clean(event.Name),
	}
}
