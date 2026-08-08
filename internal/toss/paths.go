package toss

const (
	// BaseURLReal is the Toss Securities production Open API domain.
	BaseURLReal = "https://openapi.tossinvest.com"
)

const (
	PathOAuthToken       = "/oauth2/token" //nolint:gosec // G101: URL path constant, not a credential
	PathAccounts         = "/api/v1/accounts"
	PathPrices           = "/api/v1/prices"
	PathCandles          = "/api/v1/candles"
	PathStocks           = "/api/v1/stocks"
	PathHoldings         = "/api/v1/holdings"
	PathOrders           = "/api/v1/orders"
	PathBuyingPower      = "/api/v1/buying-power"
	PathSellableQuantity = "/api/v1/sellable-quantity"
	PathCommissions      = "/api/v1/commissions"
)
