package filetoken

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/smallfish06/krsec/internal/ratelimit"
)

// Entry represents one cached token record.
type Entry struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	AppKey      string    `json:"app_key"`
}

// Options configures a file-backed token manager.
type Options struct {
	Dir                 string
	AuthLimiterName     string
	ValidityBuffer      time.Duration
	AllowFileName       func(name string) bool
	BuildFileName       func(appKey string) string
	RequireAppKeyOnLoad bool
}

// Manager stores tokens in memory and persists them on disk.
type Manager struct {
	mu     sync.RWMutex
	tokens map[string]*Entry

	authLimiters map[string]*ratelimit.Limiter
	authMu       sync.Mutex

	dir                 string
	authLimiterName     string
	validityBuffer      time.Duration
	allowFileName       func(name string) bool
	buildFileName       func(appKey string) string
	requireAppKeyOnLoad bool
}

// New creates a configured file-backed token manager.
func New(opts Options) *Manager {
	m := &Manager{
		tokens:              make(map[string]*Entry),
		authLimiters:        make(map[string]*ratelimit.Limiter),
		dir:                 strings.TrimSpace(opts.Dir),
		authLimiterName:     strings.TrimSpace(opts.AuthLimiterName),
		validityBuffer:      opts.ValidityBuffer,
		allowFileName:       opts.AllowFileName,
		buildFileName:       opts.BuildFileName,
		requireAppKeyOnLoad: opts.RequireAppKeyOnLoad,
	}
	if m.authLimiterName == "" {
		m.authLimiterName = "auth"
	}
	if m.validityBuffer < 0 {
		m.validityBuffer = 0
	}
	if m.allowFileName == nil {
		m.allowFileName = JSONFileOnly
	}
	if m.buildFileName == nil {
		m.buildFileName = DefaultHashedFileName
	}
	m.loadAll()
	return m
}

// GetToken returns a valid cached token for appKey when available.
func (m *Manager) GetToken(appKey string) (string, time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.tokens[appKey]
	if !ok {
		return "", time.Time{}, false
	}
	if time.Now().After(entry.ExpiresAt.Add(-m.validityBuffer)) {
		return "", time.Time{}, false
	}
	return entry.AccessToken, entry.ExpiresAt, true
}

// SetToken stores the token in memory and on disk.
func (m *Manager) SetToken(appKey, token string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := &Entry{
		AccessToken: token,
		ExpiresAt:   expiresAt,
		AppKey:      appKey,
	}
	m.tokens[appKey] = entry
	return m.save(appKey, entry)
}

// DeleteToken removes the cached token for appKey from memory and disk.
func (m *Manager) DeleteToken(appKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.tokens, appKey)

	dir, err := m.TokenDir()
	if err != nil {
		return nil // memory cleared, disk is best-effort
	}
	filename := strings.TrimSpace(m.buildFileName(appKey))
	if filename == "" {
		filename = DefaultHashedFileName(appKey)
	}
	path := filepath.Join(dir, filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove token file: %w", err)
	}
	return nil
}

// WaitForAuth enforces per-appKey token issuance throttling.
func (m *Manager) WaitForAuth(appKey string) {
	m.authMu.Lock()
	limiter, ok := m.authLimiters[appKey]
	if !ok {
		limiter = ratelimit.New(m.authLimiterName, 1.0/60.0, 1)
		m.authLimiters[appKey] = limiter
	}
	m.authMu.Unlock()
	_ = limiter.Wait(context.Background())
}

// TokenDir resolves the directory where token files are stored.
func (m *Manager) TokenDir() (string, error) {
	if m.dir != "" {
		return m.dir, nil
	}
	cwd, _ := os.Getwd()
	return DefaultTokenDir(cwd)
}

func (m *Manager) loadAll() {
	dir, err := m.TokenDir()
	if err != nil {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !m.allowFileName(name) {
			continue
		}

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var te Entry
		if err := json.Unmarshal(data, &te); err != nil {
			continue
		}
		if m.requireAppKeyOnLoad && strings.TrimSpace(te.AppKey) == "" {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(te.ExpiresAt.Add(-m.validityBuffer)) {
			_ = os.Remove(path)
			continue
		}
		m.tokens[te.AppKey] = &te
	}
}

func (m *Manager) save(appKey string, entry *Entry) error {
	dir, err := m.TokenDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create token dir: %w", err)
	}

	filename := strings.TrimSpace(m.buildFileName(appKey))
	if filename == "" {
		filename = DefaultHashedFileName(appKey)
	}
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token entry: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}
	return nil
}

// JSONFileOnly returns true for *.json files.
func JSONFileOnly(name string) bool {
	return filepath.Ext(name) == ".json"
}

// PrefixedJSONFileOnly returns a file predicate for prefixed *.json files.
func PrefixedJSONFileOnly(prefix string) func(string) bool {
	prefix = strings.TrimSpace(prefix)
	return func(name string) bool {
		return strings.HasPrefix(name, prefix) && JSONFileOnly(name)
	}
}

// DefaultHashedFileName returns "<hash>.json" for appKey.
func DefaultHashedFileName(appKey string) string {
	return HashAppKey(appKey) + ".json"
}

// PrefixedHashedFileName returns "<prefix><hash>.json" builder.
func PrefixedHashedFileName(prefix string) func(string) string {
	prefix = strings.TrimSpace(prefix)
	return func(appKey string) string {
		return prefix + HashAppKey(appKey) + ".json"
	}
}

// DefaultTokenDir resolves the default token directory.
func DefaultTokenDir(cwd string) (string, error) {
	if strings.TrimSpace(cwd) != "" {
		if root, ok := FindProjectRoot(cwd); ok {
			return filepath.Join(root, ".krsec", "tokens"), nil
		}
	}

	cacheDir, err := os.UserCacheDir()
	if err == nil && strings.TrimSpace(cacheDir) != "" {
		return filepath.Join(cacheDir, "krsec", "tokens"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home dir: %w", err)
	}
	return filepath.Join(home, ".krsec", "tokens"), nil
}

// FindProjectRoot walks up from start and returns the directory containing go.mod.
func FindProjectRoot(start string) (string, bool) {
	current := start
	for {
		if st, err := os.Stat(filepath.Join(current, "go.mod")); err == nil && !st.IsDir() {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

// HashAppKey returns the first 12 chars of sha256(appKey).
func HashAppKey(appKey string) string {
	sum := sha256.Sum256([]byte(appKey))
	return hex.EncodeToString(sum[:])[:12]
}
