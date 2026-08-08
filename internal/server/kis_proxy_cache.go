package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/sync/singleflight"
)

const (
	defaultKISProxyCacheMaxEntries = 2048
	kisProxyCacheStaleRetention    = 15 * time.Minute
	kisProxyCacheRealTimeTTL       = time.Minute
	kisProxyCacheCurrentPriceTTL   = 5 * time.Minute
	kisProxyCacheDailyTTL          = time.Hour
	kisProxyCacheReferenceTTL      = 6 * time.Hour
)

// KISProxyCacheRequest describes a normalized KIS proxy request for cache policy
// decisions. Params is a copy and can be inspected or modified by policy code.
type KISProxyCacheRequest struct {
	Method    string
	Path      string
	TRID      string
	AccountID string
	Params    map[string]string
}

// KISProxyCachePolicy decides whether a KIS proxy request should be cached and
// returns its soft TTL when cacheable.
type KISProxyCachePolicy func(KISProxyCacheRequest) (time.Duration, bool)

// KISProxyCacheOptions configures the in-process KIS proxy cache.
type KISProxyCacheOptions struct {
	Policy         KISProxyCachePolicy
	MaxEntries     int
	StaleRetention time.Duration
}

type kisProxyCache struct {
	items          *ttlcache.Cache[string, kisProxyCacheEntry]
	group          singleflight.Group
	staleRetention time.Duration
	now            func() time.Time
}

type kisProxyCacheEntry struct {
	value         any
	cachedAt      time.Time
	softExpiresAt time.Time
}

func newKISProxyCache(opts KISProxyCacheOptions) *kisProxyCache {
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = defaultKISProxyCacheMaxEntries
	}
	if opts.StaleRetention <= 0 {
		opts.StaleRetention = kisProxyCacheStaleRetention
	}
	c := &kisProxyCache{
		items: ttlcache.New(
			ttlcache.WithCapacity[string, kisProxyCacheEntry](uint64(opts.MaxEntries)),
			ttlcache.WithDisableTouchOnHit[string, kisProxyCacheEntry](),
		),
		staleRetention: opts.StaleRetention,
		now:            time.Now,
	}
	// Actively reclaim expired entries on a timer. Without this, ttlcache only
	// drops items on capacity pressure or on a subsequent Get of the same key,
	// so cached responses (large chart/daily payloads) linger for their full
	// ttl+staleRetention lifetime and memory never returns to baseline. Start
	// blocks until Stop, so it runs for the process lifetime in a goroutine.
	go c.items.Start()
	return c
}

func (c *kisProxyCache) getFresh(key string) (any, bool) {
	if c == nil || key == "" {
		return nil, false
	}

	item := c.items.Get(key)
	if item == nil {
		return nil, false
	}
	entry := item.Value()
	if !c.now().Before(entry.softExpiresAt) {
		return nil, false
	}
	return entry.value, true
}

func (c *kisProxyCache) getStale(key string) (any, bool) {
	if c == nil || key == "" {
		return nil, false
	}

	item := c.items.Get(key)
	if item == nil {
		return nil, false
	}
	return item.Value().value, true
}

func (c *kisProxyCache) set(key string, value any, ttl time.Duration) {
	if c == nil || key == "" || ttl <= 0 || value == nil {
		return
	}

	now := c.now()
	c.items.Set(key, kisProxyCacheEntry{
		value:         value,
		cachedAt:      now,
		softExpiresAt: now.Add(ttl),
	}, ttl+c.staleRetention)
}

func (c *kisProxyCache) do(key string, fn func() (any, error)) (any, error, bool) {
	if c == nil || key == "" {
		value, err := fn()
		return value, err, false
	}

	value, err, shared := c.group.Do(key, fn)
	return value, err, shared
}

// DefaultKISProxyCachePolicy caches public KIS quotation/reference endpoints
// and excludes trading, order, and auth-style endpoints.
func DefaultKISProxyCachePolicy(req KISProxyCacheRequest) (time.Duration, bool) {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method != "" && method != http.MethodGet && method != http.MethodPost {
		return 0, false
	}

	normalizedPath := strings.ToLower(strings.TrimSpace(req.Path))
	if !strings.HasPrefix(normalizedPath, "/uapi/") {
		return 0, false
	}
	if strings.Contains(normalizedPath, "/trading/") ||
		strings.Contains(normalizedPath, "/oauth") ||
		strings.Contains(normalizedPath, "order") {
		return 0, false
	}

	if strings.Contains(normalizedPath, "/domestic-bond/v1/quotations/") {
		return kisProxyCacheReferenceTTL, true
	}
	if strings.Contains(normalizedPath, "/ksdinfo/") || strings.Contains(normalizedPath, "holiday") {
		return kisProxyCacheReferenceTTL, true
	}
	if strings.Contains(normalizedPath, "daily") ||
		strings.Contains(normalizedPath, "chart") ||
		strings.Contains(normalizedPath, "trend") {
		if strings.Contains(normalizedPath, "time") || strings.Contains(normalizedPath, "today") {
			return kisProxyCacheRealTimeTTL, true
		}
		return kisProxyCacheDailyTTL, true
	}
	if strings.Contains(normalizedPath, "/ranking/") {
		return kisProxyCacheRealTimeTTL, true
	}
	if strings.Contains(normalizedPath, "/quotations/inquire-price") ||
		strings.HasSuffix(normalizedPath, "/quotations/price") {
		return kisProxyCacheCurrentPriceTTL, true
	}
	if strings.Contains(normalizedPath, "/quotations/") {
		return kisProxyCacheRealTimeTTL, true
	}

	return 0, false
}

func buildKISProxyCacheKey(accountID string, method string, path string, trID string, request map[string]string) string {
	payload := struct {
		AccountID string            `json:"account_id"`
		Method    string            `json:"method"`
		Path      string            `json:"path"`
		TRID      string            `json:"tr_id"`
		Request   map[string]string `json:"request,omitempty"`
	}{
		AccountID: strings.TrimSpace(accountID),
		Method:    strings.ToUpper(strings.TrimSpace(method)),
		Path:      strings.TrimSpace(path),
		TRID:      strings.TrimSpace(trID),
		Request:   request,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return "kis:" + hex.EncodeToString(sum[:])
}
