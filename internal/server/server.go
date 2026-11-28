package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
)

// Server is the krsec HTTP gateway: the routes, the broker adapter bound to
// each account, and the KIS proxy cache and rate limiter.
type Server struct {
	router         *fuego.Server
	brokers        map[string]broker.Broker // account_id -> broker adapter
	accounts       []config.AccountConfig
	logger         *slog.Logger
	apiKey         string
	kisCache       *kisProxyCache
	kisCachePolicy KISProxyCachePolicy
	kisRateLimiter *kisProxyRateLimiter

	// lifecycle scopes background work (startup authentication, symbol
	// reloads) to the server. Close cancels it.
	lifecycle context.Context
	stop      context.CancelFunc
	closeOnce sync.Once
}

// ServerOptions customizes a Server beyond what the config file carries.
type ServerOptions struct {
	Logger *slog.Logger
	// APIKey, when set, is required on every request except /health and the
	// Swagger UI. It overrides server.api_key from the config.
	APIKey            string
	KISProxyCache     KISProxyCacheOptions
	KISProxyRateLimit KISProxyRateLimitOptions
}

func newBaseServer(cfg *config.Config) *Server {
	return newBaseServerWithOptions(cfg, ServerOptions{})
}

func newBaseServerWithOptions(cfg *config.Config, opts ServerOptions) *Server {
	host := strings.TrimSpace(cfg.Server.Host)
	if host == "" {
		host = config.DefaultHost
	}
	port := cfg.Server.Port
	if port <= 0 {
		port = config.DefaultPort
	}

	apiKey := strings.TrimSpace(opts.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.Server.APIKey)
	}

	serverOpts := []fuego.ServerOption{
		fuego.WithAddr(net.JoinHostPort(host, fmt.Sprint(port))),
		fuego.WithEngineOptions(
			fuego.WithOpenAPIConfig(fuego.OpenAPIConfig{
				PrettyFormatJSON: true,
				// The spec is exported explicitly via cmd/openapi-export;
				// serving must not write doc/openapi.json into the process cwd.
				DisableLocalSave: true,
				Info: &openapi3.Info{
					Title:       "Korea Securities API",
					Description: "Unified broker API over multiple securities broker adapters",
					Version:     "1.0.0",
				},
			}),
		),
	}
	if apiKey != "" {
		serverOpts = append(serverOpts, apiKeyServerOptions()...)
	}

	rateLimitOpts := opts.KISProxyRateLimit
	if rateLimitOpts.isZero() {
		rateLimitOpts = kisProxyRateLimitOptionsFromConfig(cfg.KISProxy.RateLimit)
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	cachePolicy := opts.KISProxyCache.Policy
	if cachePolicy == nil {
		cachePolicy = DefaultKISProxyCachePolicy
	}
	lifecycle, stop := context.WithCancel(context.Background())

	s := &Server{
		router:         fuego.NewServer(serverOpts...),
		brokers:        make(map[string]broker.Broker),
		accounts:       cfg.Accounts,
		logger:         logger,
		apiKey:         apiKey,
		kisCache:       newKISProxyCache(opts.KISProxyCache),
		kisCachePolicy: cachePolicy,
		kisRateLimiter: newKISProxyRateLimiter(rateLimitOpts),
		lifecycle:      lifecycle,
		stop:           stop,
	}

	// Registered as a route middleware rather than a global one: fuego only
	// installs global middlewares inside Run, which would leave the key
	// unenforced for embedders that serve App().Mux themselves.
	if apiKey != "" {
		fuego.Use(s.router, requireAPIKey(apiKey))
	}
	s.routes()
	return s
}

// NewWithLogger creates a new server instance with a custom logger.
func NewWithLogger(cfg *config.Config, logger *slog.Logger) *Server {
	s := newBaseServerWithOptions(cfg, ServerOptions{Logger: logger})
	return s.init(cfg)
}

// New creates a new server instance.
// This constructor wires built-in brokers from config (currently KIS, Kiwoom, LS, Toss).
func New(cfg *config.Config) *Server {
	s := newBaseServer(cfg)
	return s.init(cfg)
}

// NewWithBrokers creates a server with externally provided brokers.
// This constructor is used by the public pkg/server package for OSS extensibility.
func NewWithBrokers(host string, port int, accounts []config.AccountConfig, brokers map[string]broker.Broker) *Server {
	return NewWithBrokersOptions(host, port, accounts, brokers, ServerOptions{})
}

// NewWithBrokersOptions is NewWithBrokers with a custom logger, API key and
// KIS proxy cache/rate-limit settings.
func NewWithBrokersOptions(
	host string,
	port int,
	accounts []config.AccountConfig,
	brokers map[string]broker.Broker,
	opts ServerOptions,
) *Server {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: host,
			Port: port,
		},
		Accounts: accounts,
	}
	s := newBaseServerWithOptions(cfg, opts)
	for accountID, brk := range brokers {
		if brk == nil {
			continue
		}
		s.brokers[accountID] = brk
	}
	return s
}

// shutdownDrainTimeout bounds how long in-flight requests may drain after a
// termination signal. Keep it below Kubernetes' terminationGracePeriodSeconds
// so draining finishes before SIGKILL.
const shutdownDrainTimeout = 20 * time.Second

// Run starts the HTTP server and shuts it down gracefully on SIGTERM/SIGINT,
// draining in-flight requests instead of dropping them mid-rollout.
func (s *Server) Run() error {
	defer s.Close()

	if s.apiKey == "" && !isLoopbackAddr(s.router.Addr) {
		s.logger.Warn("gateway accepts unauthenticated requests on a non-loopback address; set server.api_key",
			"addr", s.router.Addr)
	}
	s.logger.Info("server listening", "addr", s.router.Addr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.router.Run()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		s.logger.Info("shutting down", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
		defer cancel()
		if err := s.router.Shutdown(ctx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		// Serve returns http.ErrServerClosed after Shutdown; not an error.
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// Close stops the server's background work: startup authentication, KIS
// symbol reloads and the proxy cache janitor. Run calls it on return; callers
// that serve through App instead of Run should call it themselves.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.stop()
		s.kisCache.stop()
	})
}

// App returns the underlying Fuego server for embedding/customization.
func (s *Server) App() *fuego.Server {
	return s.router
}

// HealthResponse is the response body for the health check endpoint.
type HealthResponse struct {
	Status   string `json:"status"`
	Accounts int    `json:"accounts"`
}

func (s *Server) handleHealth(c fuego.ContextNoBody) (HealthResponse, error) {
	c.SetStatus(http.StatusOK)
	return HealthResponse{
		Status:   "ok",
		Accounts: len(s.accounts),
	}, nil
}

// Response is the envelope every krsec endpoint returns.
type Response struct {
	OK     bool   `json:"ok"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
	Broker string `json:"broker,omitempty"`
}

func respond(c interface{ SetStatus(int) }, status int, data Response) (Response, error) {
	c.SetStatus(status)
	return data, nil
}
