package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
)

// Server represents the HTTP server
type Server struct {
	config         *config.Config
	router         *fuego.Server
	brokers        map[string]broker.Broker // account_id -> broker adapter
	accounts       []config.AccountConfig
	logger         *slog.Logger
	kisCache       *kisProxyCache
	kisCachePolicy KISProxyCachePolicy
	kisRateLimiter *kisProxyRateLimiter
}

type ServerOptions struct {
	Logger            *slog.Logger
	KISProxyCache     KISProxyCacheOptions
	KISProxyRateLimit KISProxyRateLimitOptions
}

func newBaseServer(cfg *config.Config) *Server {
	return newBaseServerWithOptions(cfg, ServerOptions{})
}

func newBaseServerWithOptions(cfg *config.Config, opts ServerOptions) *Server {
	host := strings.TrimSpace(cfg.Server.Host)
	if host == "" {
		host = "localhost"
	}
	port := cfg.Server.Port
	if port <= 0 {
		port = 8080
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	r := fuego.NewServer(
		fuego.WithAddr(addr),
		fuego.WithEngineOptions(
			fuego.WithOpenAPIConfig(fuego.OpenAPIConfig{
				PrettyFormatJSON: true,
				Info: &openapi3.Info{
					Title:       "Korea Securities API",
					Description: "Unified broker API over multiple securities broker adapters",
					Version:     "1.0.0",
				},
			}),
		),
	)

	rateLimitOpts := opts.KISProxyRateLimit
	if rateLimitOpts.isZero() {
		rateLimitOpts = kisProxyRateLimitOptionsFromConfig(cfg.KISProxy.RateLimit)
	}

	s := &Server{
		config:         cfg,
		router:         r,
		brokers:        make(map[string]broker.Broker),
		accounts:       cfg.Accounts,
		logger:         slog.Default(),
		kisCache:       newKISProxyCache(opts.KISProxyCache),
		kisCachePolicy: opts.KISProxyCache.Policy,
		kisRateLimiter: newKISProxyRateLimiter(rateLimitOpts),
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if opts.Logger != nil {
		s.logger = opts.Logger
	}
	if s.kisCachePolicy == nil {
		s.kisCachePolicy = DefaultKISProxyCachePolicy
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

func NewWithBrokersOptions(
	host string,
	port int,
	accounts []config.AccountConfig,
	brokers map[string]broker.Broker,
	opts ServerOptions,
) *Server {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "localhost"
	}
	if port <= 0 {
		port = 8080
	}

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

// App returns the underlying Fuego server for embedding/customization.
func (s *Server) App() *fuego.Server {
	return s.router
}

// handleHealth handles health check requests
func (s *Server) handleHealth(c fuego.ContextNoBody) (map[string]any, error) {
	c.SetStatus(http.StatusOK)
	return map[string]any{
		"status":   "ok",
		"accounts": len(s.accounts),
	}, nil
}

// getBroker returns the broker for the given account ID
func (s *Server) getBroker(accountID string) broker.Broker {
	if brk, status, _ := s.resolveBrokerByAccountID(accountID); status == 0 {
		return brk
	}
	return nil
}

// getFirstBroker returns the first available broker (for legacy endpoints)
func (s *Server) getFirstBroker() broker.Broker {
	if len(s.accounts) > 0 {
		if brk := s.getBroker(s.accounts[0].AccountID); brk != nil {
			return brk
		}
	}
	if len(s.brokers) == 0 {
		return nil
	}
	ids := make([]string, 0, len(s.brokers))
	for accountID := range s.brokers {
		ids = append(ids, accountID)
	}
	sort.Strings(ids)
	for _, accountID := range ids {
		if brk := s.brokers[accountID]; brk != nil {
			return brk
		}
	}
	return nil
}

// Response represents a standard API response
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
