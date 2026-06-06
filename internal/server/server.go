package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/internal/kis"
	kisadapter "github.com/smallfish06/krsec/internal/kis/adapter"
	"github.com/smallfish06/krsec/internal/kiwoom"
	kiwoomadapter "github.com/smallfish06/krsec/internal/kiwoom/adapter"
	"github.com/smallfish06/krsec/internal/ls"
	lsadapter "github.com/smallfish06/krsec/internal/ls/adapter"
	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
	kisspecs "github.com/smallfish06/krsec/pkg/kis/specs"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
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
// This constructor wires built-in brokers from config (currently KIS, Kiwoom, LS).
func New(cfg *config.Config) *Server {
	s := newBaseServer(cfg)
	return s.init(cfg)
}

func (s *Server) init(cfg *config.Config) *Server {

	kisTokenManager := kis.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	kiwoomTokenManager := kiwoom.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	lsTokenManager := ls.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)

	// Initialize brokers for each account
	for _, account := range cfg.Accounts {
		var brk broker.Broker
		switch account.Broker {
		case broker.CodeKIS:
			adapter := kisadapter.NewAdapterWithOptions(
				account.Sandbox,
				account.AccountID,
				kisTokenManager,
				cfg.Storage.OrderContextDir,
				s.logger,
			)
			creds := broker.Credentials{
				AppKey:    account.AppKey,
				AppSecret: account.AppSecret,
			}
			// Authenticate in background (don't block server start)
			go func(name string, a *kisadapter.Adapter, c broker.Credentials, logger *slog.Logger) {
				if _, err := a.Authenticate(context.Background(), c); err != nil {
					logger.Warn("failed to authenticate account", "account", name, "error", err)
				} else {
					logger.Info("authenticated account", "account", name)
				}
			}(account.Name, adapter, creds, s.logger)

			// Bootstrap symbol master files in background.
			go func(name string, a *kisadapter.Adapter, logger *slog.Logger) {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				count, err := a.BootstrapSymbols(ctx)
				if err != nil {
					logger.Warn("symbol bootstrap failed", "account", name, "error", err)
				} else {
					logger.Info("bootstrapped symbol records", "account", name, "count", count)
				}

				// Keep symbol master cache fresh (KIS master files change over time).
				ticker := time.NewTicker(24 * time.Hour)
				defer ticker.Stop()
				for range ticker.C {
					reloadCtx, reloadCancel := context.WithTimeout(context.Background(), 90*time.Second)
					count, err := a.ReloadSymbols(reloadCtx)
					reloadCancel()
					if err != nil {
						logger.Warn("symbol reload failed", "account", name, "error", err)
						continue
					}
					logger.Info("reloaded symbol records", "account", name, "count", count)
				}
			}(account.Name, adapter, s.logger)
			brk = adapter
		case broker.CodeKiwoom:
			adapter := kiwoomadapter.NewAdapterWithOptions(
				account.Sandbox,
				account.AccountID,
				kiwoomTokenManager,
				cfg.Storage.OrderContextDir,
				s.logger,
			)
			creds := broker.Credentials{
				AppKey:    account.AppKey,
				AppSecret: account.AppSecret,
			}
			go func(name string, a *kiwoomadapter.Adapter, c broker.Credentials, logger *slog.Logger) {
				if _, err := a.Authenticate(context.Background(), c); err != nil {
					logger.Warn("failed to authenticate account", "account", name, "error", err)
				} else {
					logger.Info("authenticated account", "account", name)
				}
			}(account.Name, adapter, creds, s.logger)
			brk = adapter
		case broker.CodeLS:
			adapter := lsadapter.NewAdapterWithOptions(
				account.Sandbox,
				account.AccountID,
				lsTokenManager,
				account.MACAddress,
				s.logger,
			)
			creds := broker.Credentials{
				AppKey:    account.AppKey,
				AppSecret: account.AppSecret,
			}
			go func(name string, a *lsadapter.Adapter, c broker.Credentials, logger *slog.Logger) {
				if _, err := a.Authenticate(context.Background(), c); err != nil {
					logger.Warn("failed to authenticate account", "account", name, "error", err)
				} else {
					logger.Info("authenticated account", "account", name)
				}
			}(account.Name, adapter, creds, s.logger)
			brk = adapter
		default:
			s.logger.Warn("unknown broker type", "broker", account.Broker)
			continue
		}
		s.brokers[account.AccountID] = brk
	}

	return s
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

// routes sets up HTTP routes
func (s *Server) routes() {
	fuego.Get(s.router, "/health", s.handleHealth,
		fuego.OptionTags("System"),
		fuego.OptionSummary("Health check"),
	)

	// Auth
	fuego.Post(s.router, "/auth/token", s.handleAuthToken,
		fuego.OptionTags("Auth"),
		fuego.OptionSummary("Issue broker auth token"),
		fuego.OptionDescription("Authenticate with a broker and receive an access token."),
	)

	// KIS static endpoint routes (documented KIS paths exposed as static OpenAPI operations)
	s.registerKISStaticProxyRoutes()
	// Kiwoom static endpoint routes (documented Kiwoom path+api_id exposed as static OpenAPI operations)
	s.registerKiwoomStaticProxyRoutes()

	// Kiwoom endpoint dispatcher (calls supported Kiwoom endpoints by path + api_id)
	fuego.Post(s.router, "/kiwoom/{path...}", s.handleKiwoomProxy,
		fuego.OptionTags("Kiwoom"),
		fuego.OptionSummary("Call Kiwoom endpoint by path"),
		fuego.OptionDescription("Calls Kiwoom endpoints implemented in krsec by path. api_id in request body is required."),
		fuego.OptionPath("path", "Kiwoom API path under /api. Accepts wildcard segments."),
		fuego.OptionQuery("account_id", "Optional account selector when multiple Kiwoom accounts exist."),
	)

	// LS endpoint dispatcher (calls LS endpoints by path + tr_cd)
	fuego.Post(s.router, "/ls/{path...}", s.handleLSProxy,
		fuego.OptionTags("LS"),
		fuego.OptionSummary("Call LS endpoint by path"),
		fuego.OptionDescription("Calls LS Securities OpenAPI endpoints by path. tr_cd in request body is required."),
		fuego.OptionPath("path", "LS API path, for example /stock/market-data. Accepts wildcard segments."),
		fuego.OptionQuery("account_id", "Optional account selector when multiple LS accounts exist."),
	)

	// Quotes
	fuego.Get(s.router, "/quotes/{market}/{symbol}", s.handleGetQuote,
		fuego.OptionTags("Quotes"),
		fuego.OptionSummary("Get latest quote"),
		fuego.OptionDescription("Returns the current price for a symbol."),
		fuego.OptionPath("market", "Exchange market code", fuego.ParamExample("KRX", "KRX"), fuego.ParamExample("NASDAQ", "NASDAQ")),
		fuego.OptionPath("symbol", "Ticker symbol", fuego.ParamExample("Samsung", "005930"), fuego.ParamExample("AAPL", "AAPL")),
		fuego.OptionQuery("account_id", "Use a specific account's broker (optional)", fuego.ParamExample("KIS account", "12345678-01")),
	)

	fuego.Get(s.router, "/quotes/{market}/{symbol}/ohlcv", s.handleGetOHLCV,
		fuego.OptionTags("Quotes"),
		fuego.OptionSummary("Get OHLCV candles"),
		fuego.OptionDescription("Returns daily/weekly/monthly candlestick data."),
		fuego.OptionPath("market", "Exchange market code", fuego.ParamExample("KRX", "KRX")),
		fuego.OptionPath("symbol", "Ticker symbol", fuego.ParamExample("Samsung", "005930")),
		fuego.OptionQuery("account_id", "Use a specific account's broker (optional)", fuego.ParamExample("KIS account", "12345678-01")),
		fuego.OptionQuery("interval", "Candle interval: 1d, 1w, 1mo", fuego.ParamDefault("1d"), fuego.ParamExample("daily", "1d"), fuego.ParamExample("weekly", "1w")),
		fuego.OptionQuery("from", "Start date (YYYY-MM-DD)", fuego.ParamExample("Jan 2026", "2026-01-01")),
		fuego.OptionQuery("to", "End date (YYYY-MM-DD)", fuego.ParamExample("Feb 2026", "2026-02-28")),
		fuego.OptionQuery("limit", "Max number of candles (default 100, max 2000)", fuego.ParamDefault("100")),
	)

	// Instruments
	fuego.Get(s.router, "/instruments/{market}/{symbol}", s.handleGetInstrument,
		fuego.OptionTags("Instruments"),
		fuego.OptionSummary("Get instrument metadata"),
		fuego.OptionDescription("Returns metadata for a symbol: name, ISIN, sector, listing status, etc."),
		fuego.OptionPath("market", "Exchange market code", fuego.ParamExample("KRX", "KRX")),
		fuego.OptionPath("symbol", "Ticker symbol", fuego.ParamExample("Samsung", "005930")),
		fuego.OptionQuery("account_id", "Use a specific account's broker (optional)"),
	)

	// Accounts
	fuego.Get(s.router, "/accounts", s.handleListAccounts,
		fuego.OptionTags("Accounts"),
		fuego.OptionSummary("List configured accounts"),
	)

	fuego.Get(s.router, "/accounts/summary", s.handleAccountsSummary,
		fuego.OptionTags("Accounts"),
		fuego.OptionSummary("Get combined account summary"),
		fuego.OptionDescription("Aggregated balance across all configured accounts."),
	)

	fuego.Get(s.router, "/accounts/{account_id}/balance", s.handleGetBalance,
		fuego.OptionTags("Accounts"),
		fuego.OptionSummary("Get account balance"),
		fuego.OptionPath("account_id", "Account ID", fuego.ParamExample("KIS", "12345678-01"), fuego.ParamExample("Kiwoom", "1234567890")),
	)

	fuego.Get(s.router, "/accounts/{account_id}/positions", s.handleGetPositions,
		fuego.OptionTags("Accounts"),
		fuego.OptionSummary("Get account positions"),
		fuego.OptionPath("account_id", "Account ID", fuego.ParamExample("KIS", "12345678-01")),
	)

	// Orders (account-scoped)
	fuego.Get(s.router, "/accounts/{account_id}/orders/{order_id}/fills", s.handleGetOrderFills,
		fuego.OptionTags("Orders"),
		fuego.OptionSummary("Get order fills"),
		fuego.OptionPath("account_id", "Account that placed the order"),
		fuego.OptionPath("order_id", "Order ID returned from place order"),
	)

	fuego.Get(s.router, "/accounts/{account_id}/orders/{order_id}", s.handleGetOrder,
		fuego.OptionTags("Orders"),
		fuego.OptionSummary("Get order status"),
		fuego.OptionPath("account_id", "Account that placed the order"),
		fuego.OptionPath("order_id", "Order ID"),
	)

	fuego.Post(s.router, "/accounts/{account_id}/orders", s.handlePlaceOrder,
		fuego.OptionTags("Orders"),
		fuego.OptionSummary("Place order"),
		fuego.OptionDescription("Submit a new buy or sell order."),
		fuego.OptionPath("account_id", "Account ID", fuego.ParamExample("KIS", "12345678-01"), fuego.ParamExample("Kiwoom", "1234567890")),
	)

	fuego.Delete(s.router, "/accounts/{account_id}/orders/{order_id}", s.handleCancelOrder,
		fuego.OptionTags("Orders"),
		fuego.OptionSummary("Cancel order"),
		fuego.OptionPath("account_id", "Account that placed the order"),
		fuego.OptionPath("order_id", "Order ID to cancel"),
	)

	fuego.Put(s.router, "/accounts/{account_id}/orders/{order_id}", s.handleModifyOrder,
		fuego.OptionTags("Orders"),
		fuego.OptionSummary("Modify order"),
		fuego.OptionDescription("Change price or quantity of a pending order."),
		fuego.OptionPath("account_id", "Account that placed the order"),
		fuego.OptionPath("order_id", "Order ID to modify"),
	)
}

func (s *Server) registerKISStaticProxyRoutes() {
	uapiPaths := make([]string, 0, len(kisspecs.DocumentedKISEndpointSpecs))
	for p := range kisspecs.DocumentedKISEndpointSpecs {
		uapiPaths = append(uapiPaths, p)
	}
	sort.Strings(uapiPaths)

	for _, uapiPath := range uapiPaths {
		spec := kisspecs.DocumentedKISEndpointSpecs[uapiPath]
		proxyPath := toKISStaticProxyPath(uapiPath)
		if proxyPath == "" {
			continue
		}

		desc := fmt.Sprintf("Static documented KIS proxy route for %s %s.", strings.ToUpper(strings.TrimSpace(spec.Method)), uapiPath)
		summary := "Call KIS static endpoint " + proxyPath

		options := []fuego.RouteOption{
			fuego.OptionTags("KIS"),
			fuego.OptionSummary(summary),
			fuego.OptionDescription(desc),
			fuego.OptionQuery("account_id", "Optional account selector when multiple KIS accounts exist."),
		}

		if reqType := kisspecs.NewDocumentedEndpointRequest(uapiPath); reqType != nil {
			options = append(options, fuego.OptionRequestBody(fuego.RequestBody{
				Type:         reqType,
				ContentTypes: []string{"application/json"},
			}))
		}
		if respType := kisspecs.NewDocumentedEndpointResponse(uapiPath); respType != nil {
			options = append(options, fuego.OptionAddResponse(http.StatusOK, "OK", fuego.Response{
				Type:         respType,
				ContentTypes: []string{"application/json"},
			}))
		}

		fuego.Post(s.router, proxyPath, s.handleKISProxyStatic(uapiPath), options...)
	}
}

func (s *Server) registerKiwoomStaticProxyRoutes() {
	keys := make([]string, 0, len(kiwoomspecs.DocumentedKiwoomEndpointSpecs))
	for key := range kiwoomspecs.DocumentedKiwoomEndpointSpecs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		spec := kiwoomspecs.DocumentedKiwoomEndpointSpecs[key]
		proxyPath := toKiwoomStaticProxyPath(spec.Path, spec.APIID)
		if proxyPath == "" {
			continue
		}

		desc := fmt.Sprintf(
			"Static documented Kiwoom proxy route for %s %s (api_id=%s).",
			strings.ToUpper(strings.TrimSpace(spec.Method)),
			spec.Path,
			spec.APIID,
		)
		summary := "Call Kiwoom static endpoint " + proxyPath

		options := []fuego.RouteOption{
			fuego.OptionTags("Kiwoom"),
			fuego.OptionSummary(summary),
			fuego.OptionDescription(desc),
			fuego.OptionQuery("account_id", "Optional account selector when multiple Kiwoom accounts exist."),
		}

		if reqType := kiwoomspecs.NewDocumentedEndpointRequest(spec.Path, spec.APIID); reqType != nil {
			options = append(options, fuego.OptionRequestBody(fuego.RequestBody{
				Type:         reqType,
				ContentTypes: []string{"application/json"},
			}))
		}
		if respType := kiwoomspecs.NewDocumentedEndpointResponse(spec.Path, spec.APIID); respType != nil {
			options = append(options, fuego.OptionAddResponse(http.StatusOK, "OK", fuego.Response{
				Type:         respType,
				ContentTypes: []string{"application/json"},
			}))
		}

		fuego.Post(s.router, proxyPath, s.handleKiwoomProxyStatic(spec.Path, spec.APIID), options...)
	}
}

func toKISStaticProxyPath(uapiPath string) string {
	p := strings.TrimSpace(uapiPath)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasPrefix(p, kis.PathPrefixUAPISlash) {
		return ""
	}
	trimmed := strings.TrimPrefix(p, kis.PathPrefixUAPI)
	if trimmed == "" || trimmed == "/" {
		return ""
	}
	return "/kis" + trimmed
}

func toKiwoomStaticProxyPath(path, apiID string) string {
	p := strings.TrimSpace(path)
	id := strings.ToLower(strings.TrimSpace(apiID))
	if p == "" || id == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if !strings.HasPrefix(p, kiwoom.PathPrefixAPISlash) {
		return ""
	}

	trimmed := strings.TrimPrefix(p, kiwoom.PathPrefixAPI)
	if trimmed == "" || trimmed == "/" {
		return ""
	}
	return "/kiwoom" + trimmed + "/" + id
}

// Run starts the HTTP server
func (s *Server) Run() error {
	s.logger.Info("server listening", "addr", s.router.Addr)
	return s.router.Run()
}

// App returns the underlying Fuego server for embedding/customization.
func (s *Server) App() *fuego.Server {
	return s.router
}

// handleHealth handles health check requests
func (s *Server) handleHealth(c fuego.ContextNoBody) (map[string]interface{}, error) {
	c.SetStatus(http.StatusOK)
	return map[string]interface{}{
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
	OK     bool        `json:"ok"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
	Broker string      `json:"broker,omitempty"`
}

func respond(c interface{ SetStatus(int) }, status int, data Response) (Response, error) {
	c.SetStatus(status)
	return data, nil
}
