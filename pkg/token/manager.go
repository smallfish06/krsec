// Package token defines the broker-agnostic access-token manager contract.
package token

import "time"

// Manager caches broker access tokens per app key and throttles how often a
// new one may be issued.
type Manager interface {
	// GetToken returns the cached token while it is still usable.
	GetToken(appKey string) (token string, expiresAt time.Time, ok bool)
	SetToken(appKey, token string, expiresAt time.Time) error
	DeleteToken(appKey string) error
	// WaitForAuth blocks until the broker's issuance limit allows another
	// token request for appKey.
	WaitForAuth(appKey string)
}

// InvalidateRejected drops the cached token for appKey after the broker
// rejected it, unless the cache already holds a different one.
//
// Several in-flight requests usually fail on the same expired token. The first
// to notice issues a replacement; the rest must not delete that replacement,
// or each of them would queue behind the issuance limit for another one.
func InvalidateRejected(m Manager, appKey, rejected string) error {
	if current, _, ok := m.GetToken(appKey); ok && current != rejected {
		return nil
	}
	return m.DeleteToken(appKey)
}
