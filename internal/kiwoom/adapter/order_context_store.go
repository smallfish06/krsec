package adapter

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/smallfish06/krsec/internal/orderctxstore"
)

const maxPersistedOrderContexts = 300

func (a *Adapter) loadOrderContexts() error {
	path, err := a.orderContextFilePath()
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return orderctxstore.Load(path, a.orders, maxPersistedOrderContexts, func(meta orderContext) time.Time {
		return meta.UpdatedAt
	})
}

func (a *Adapter) persistOrderContexts() error {
	path, err := a.orderContextFilePath()
	if err != nil {
		return err
	}

	a.mu.RLock()
	snapshot := make(map[string]orderContext, len(a.orders))
	maps.Copy(snapshot, a.orders)
	a.mu.RUnlock()
	return orderctxstore.Persist(path, snapshot)
}

func (a *Adapter) compactOrderContextsLocked(limit int) {
	orderctxstore.Compact(a.orders, limit, func(meta orderContext) time.Time {
		return meta.UpdatedAt
	})
}

func (a *Adapter) orderContextFilePath() (string, error) {
	baseDir := a.orderDir
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(home, ".krsec", "orders")
	}

	env := "real"
	if a.sandbox {
		env = "sandbox"
	}
	file := fmt.Sprintf("kiwoom-%s-%s.json", env, sanitizeFilename(a.accountID))
	return filepath.Join(baseDir, file), nil
}

func sanitizeFilename(v string) string {
	if v == "" {
		return "unknown"
	}
	out := make([]rune, 0, len(v))
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r)
		case r >= '0' && r <= '9':
			out = append(out, r)
		case r == '-' || r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
