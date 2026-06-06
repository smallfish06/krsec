package ls

import (
	internal "github.com/smallfish06/krsec/internal/ls"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

// NewFileTokenManager creates a file-backed LS token manager.
func NewFileTokenManager() tokencache.Manager {
	return internal.NewFileTokenManager()
}

// NewFileTokenManagerWithDir creates a file-backed LS token manager with a fixed directory.
func NewFileTokenManagerWithDir(dir string) tokencache.Manager {
	return internal.NewFileTokenManagerWithDir(dir)
}
