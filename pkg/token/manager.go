package token

import "time"

// Manager defines token cache and token-issuance throttling behavior.
type Manager interface {
	GetToken(appKey string) (string, time.Time, bool)
	SetToken(appKey, token string, expiresAt time.Time) error
	DeleteToken(appKey string) error
	WaitForAuth(appKey string)
}
