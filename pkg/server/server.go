package server

import (
	"log/slog"
	"strings"
	"time"

	"github.com/go-fuego/fuego"

	internalserver "github.com/smallfish06/krsec/internal/server"
	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
)

// Account describes an externally supplied account/broker binding.
type Account struct {
	ID          string
	Name        string
	Broker      string
	Sandbox     bool
	Credentials broker.Credentials
}

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

// KISProxyRateLimitOptions configures outbound KIS upstream throttling.
type KISProxyRateLimitOptions struct {
	Disabled          bool
	RequestsPerSecond float64
	Burst             int
}

// Options configures the public API server.
// External users can provide their own broker implementations through Brokers.
type Options struct {
	Host              string
	Port              int
	Accounts          []Account
	Brokers           map[string]broker.Broker // account_id -> broker implementation
	Logger            *slog.Logger             // optional structured logger; nil uses slog.Default()
	KISProxyCache     KISProxyCacheOptions
	KISProxyRateLimit KISProxyRateLimitOptions
}

// Server wraps the internal HTTP server and exposes a stable public API.
type Server struct {
	inner *internalserver.Server
}

// New creates a server with externally supplied broker implementations.
func New(opts Options) *Server {
	inner := internalserver.NewWithBrokersOptions(
		opts.Host,
		opts.Port,
		toInternalAccounts(opts.Accounts),
		opts.Brokers,
		toInternalOptions(opts),
	)
	return &Server{inner: inner}
}

// Run starts the HTTP server.
func (s *Server) Run() error {
	return s.inner.Run()
}

// App returns the underlying Fuego server for embedding or custom route composition.
func (s *Server) App() *fuego.Server {
	return s.inner.App()
}

// DefaultKISProxyCachePolicy caches public KIS quotation/reference endpoints
// and excludes trading, order, and auth-style endpoints.
func DefaultKISProxyCachePolicy(req KISProxyCacheRequest) (time.Duration, bool) {
	return internalserver.DefaultKISProxyCachePolicy(toInternalCacheRequest(req))
}

func toInternalOptions(opts Options) internalserver.ServerOptions {
	var policy internalserver.KISProxyCachePolicy
	if opts.KISProxyCache.Policy != nil {
		policy = func(req internalserver.KISProxyCacheRequest) (time.Duration, bool) {
			return opts.KISProxyCache.Policy(toPublicCacheRequest(req))
		}
	}
	return internalserver.ServerOptions{
		Logger: opts.Logger,
		KISProxyCache: internalserver.KISProxyCacheOptions{
			Policy:         policy,
			MaxEntries:     opts.KISProxyCache.MaxEntries,
			StaleRetention: opts.KISProxyCache.StaleRetention,
		},
		KISProxyRateLimit: internalserver.KISProxyRateLimitOptions{
			Disabled:          opts.KISProxyRateLimit.Disabled,
			RequestsPerSecond: opts.KISProxyRateLimit.RequestsPerSecond,
			Burst:             opts.KISProxyRateLimit.Burst,
		},
	}
}

func toInternalCacheRequest(req KISProxyCacheRequest) internalserver.KISProxyCacheRequest {
	return internalserver.KISProxyCacheRequest{
		Method:    req.Method,
		Path:      req.Path,
		TRID:      req.TRID,
		AccountID: req.AccountID,
		Params:    req.Params,
	}
}

func toPublicCacheRequest(req internalserver.KISProxyCacheRequest) KISProxyCacheRequest {
	return KISProxyCacheRequest{
		Method:    req.Method,
		Path:      req.Path,
		TRID:      req.TRID,
		AccountID: req.AccountID,
		Params:    req.Params,
	}
}

func toInternalAccounts(accounts []Account) []config.AccountConfig {
	out := make([]config.AccountConfig, 0, len(accounts))
	for _, acc := range accounts {
		id := strings.TrimSpace(acc.ID)
		if id == "" {
			continue
		}
		out = append(out, config.AccountConfig{
			Name:      strings.TrimSpace(acc.Name),
			Broker:    strings.ToLower(strings.TrimSpace(acc.Broker)),
			Sandbox:   acc.Sandbox,
			AppKey:    strings.TrimSpace(acc.Credentials.AppKey),
			AppSecret: strings.TrimSpace(acc.Credentials.AppSecret),
			AccountID: id,
		})
	}
	return out
}

// RunFromConfigFile loads a config.yaml and starts the server.
// This is the simplest way to embed krsec in another project.
func RunFromConfigFile(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	srv := internalserver.New(cfg)
	return srv.Run()
}

// RunFromConfigFileWithLogger loads a config.yaml and starts the server with a custom logger.
func RunFromConfigFileWithLogger(path string, logger *slog.Logger) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	srv := internalserver.NewWithLogger(cfg, logger)
	return srv.Run()
}
