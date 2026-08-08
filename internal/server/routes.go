package server

import (
	"github.com/go-fuego/fuego"
)

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
		fuego.OptionPath("path...", "Kiwoom API path under /api. Accepts wildcard segments."),
		fuego.OptionQuery("account_id", "Optional account selector when multiple Kiwoom accounts exist."),
	)

	// LS endpoint dispatcher (calls LS endpoints by path + tr_cd)
	fuego.Post(s.router, "/ls/{path...}", s.handleLSProxy,
		fuego.OptionTags("LS"),
		fuego.OptionSummary("Call LS endpoint by path"),
		fuego.OptionDescription("Calls LS Securities OpenAPI endpoints by path. tr_cd in request body is required."),
		fuego.OptionPath("path...", "LS API path, for example /stock/market-data. Accepts wildcard segments."),
		fuego.OptionQuery("account_id", "Optional account selector when multiple LS accounts exist."),
	)

	// Toss endpoint dispatcher (calls documented Toss endpoints by path)
	fuego.Post(s.router, "/toss/{path...}", s.handleTossProxy,
		fuego.OptionTags("Toss"),
		fuego.OptionSummary("Call Toss endpoint by path"),
		fuego.OptionDescription("Calls Toss Securities Open API endpoints by path. method is required when one path has multiple operations."),
		fuego.OptionPath("path...", "Toss API path, for example /api/v1/prices. Accepts wildcard segments."),
		fuego.OptionQuery("account_id", "Optional account selector when multiple Toss accounts exist."),
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
