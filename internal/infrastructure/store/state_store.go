package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"excelabel/internal/domain"
)

type StateStore struct {
	statePath string
}

func New(statePath string) *StateStore {
	return &StateStore{statePath: statePath}
}

func DefaultStatePath(rootDir string) string {
	return filepath.Join(rootDir, ".excelabel", "workspace-state.json")
}

func (store *StateStore) SaveWorkspace(state domain.WorkspaceState) error {
	if err := os.MkdirAll(filepath.Dir(store.statePath), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace state: %w", err)
	}

	if err := os.WriteFile(store.statePath, payload, 0o644); err != nil {
		return fmt.Errorf("write workspace state: %w", err)
	}

	return nil
}

func (store *StateStore) LoadWorkspace() (domain.WorkspaceState, error) {
	payload, err := os.ReadFile(store.statePath)
	if err != nil {
		return domain.WorkspaceState{}, fmt.Errorf("read workspace state: %w", err)
	}

	var state domain.WorkspaceState
	if err := json.Unmarshal(payload, &state); err != nil {
		return domain.WorkspaceState{}, fmt.Errorf("unmarshal workspace state: %w", err)
	}

	if state.Entries == nil {
		state.Entries = make(map[string]domain.FileEntry)
	}
	if state.PendingEvents == nil {
		state.PendingEvents = make([]domain.SyncEvent, 0)
	}

	return state, nil
}
