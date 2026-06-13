package toss

import (
	internal "github.com/smallfish06/krsec/internal/toss"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

// NewFileTokenManager creates a file-backed Toss token manager.
func NewFileTokenManager() tokencache.Manager {
	return internal.NewFileTokenManager()
}

// NewFileTokenManagerWithDir creates a file-backed Toss token manager with a fixed directory.
func NewFileTokenManagerWithDir(dir string) tokencache.Manager {
	return internal.NewFileTokenManagerWithDir(dir)
}
