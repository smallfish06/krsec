package ls

import (
	"strings"
	"sync"
	"time"

	"github.com/smallfish06/krsec/internal/filetoken"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

// FileTokenManager stores LS OAuth tokens in memory and persists them to disk.
type FileTokenManager struct {
	*filetoken.Manager
}

var (
	globalTokenManager   tokencache.Manager
	globalTokenManagerMu sync.RWMutex
)

// NewFileTokenManager creates the default file-backed token manager.
func NewFileTokenManager() *FileTokenManager {
	return NewFileTokenManagerWithDir("")
}

// NewFileTokenManagerWithDir creates a file-backed token manager with an optional fixed directory.
func NewFileTokenManagerWithDir(dir string) *FileTokenManager {
	return &FileTokenManager{
		Manager: filetoken.New(filetoken.Options{
			Dir:                 strings.TrimSpace(dir),
			AuthLimiterName:     "ls-auth",
			ValidityBuffer:      5 * time.Minute,
			AllowFileName:       filetoken.JSONFileOnly,
			BuildFileName:       filetoken.DefaultHashedFileName,
			RequireAppKeyOnLoad: false,
		}),
	}
}

// GetTokenManager returns the package-global token manager.
func GetTokenManager() tokencache.Manager {
	globalTokenManagerMu.RLock()
	tm := globalTokenManager
	globalTokenManagerMu.RUnlock()
	if tm != nil {
		return tm
	}

	globalTokenManagerMu.Lock()
	defer globalTokenManagerMu.Unlock()
	if globalTokenManager == nil {
		globalTokenManager = NewFileTokenManager()
	}
	return globalTokenManager
}
