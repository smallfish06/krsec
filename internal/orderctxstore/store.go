package orderctxstore

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const stateVersion = 1

type state[T any] struct {
	Version   int          `json:"version"`
	UpdatedAt time.Time    `json:"updated_at"`
	Orders    map[string]T `json:"orders"`
}

// Load reads persisted order contexts from path into dst and compacts to limit.
// It returns nil when the file does not exist.
func Load[T any](path string, dst map[string]T, limit int, updatedAt func(T) time.Time) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var st state[T]
	if err := json.Unmarshal(data, &st); err != nil {
		return err
	}
	if len(st.Orders) == 0 {
		return nil
	}

	maps.Copy(dst, st.Orders)
	Compact(dst, limit, updatedAt)
	return nil
}

// Persist atomically writes order contexts to disk.
func Persist[T any](path string, src map[string]T) error {
	snapshot := make(map[string]T, len(src))
	maps.Copy(snapshot, src)

	st := state[T]{
		Version:   stateVersion,
		UpdatedAt: time.Now(),
		Orders:    snapshot,
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Compact keeps only the newest limit contexts according to updatedAt.
func Compact[T any](orders map[string]T, limit int, updatedAt func(T) time.Time) {
	if limit <= 0 || len(orders) <= limit {
		return
	}

	type row struct {
		orderID   string
		updatedAt time.Time
	}
	rows := make([]row, 0, len(orders))
	for orderID, meta := range orders {
		ts := time.Time{}
		if updatedAt != nil {
			ts = updatedAt(meta)
		}
		rows = append(rows, row{orderID: orderID, updatedAt: ts})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].updatedAt.After(rows[j].updatedAt)
	})

	keep := make(map[string]struct{}, limit)
	for i := 0; i < limit && i < len(rows); i++ {
		keep[rows[i].orderID] = struct{}{}
	}
	for orderID := range orders {
		if _, ok := keep[orderID]; !ok {
			delete(orders, orderID)
		}
	}
}
