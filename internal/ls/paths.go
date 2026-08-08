package ls

const (
	// BaseURLReal is the LS Securities production REST domain.
	BaseURLReal = "https://openapi.ls-sec.co.kr:8080"
	// BaseURLSandbox is the LS Securities mock REST domain. LS currently uses
	// the same REST host and routes mock access by the issued app key.
	BaseURLSandbox = "https://openapi.ls-sec.co.kr:8080"

	// WebSocketURLReal is the LS Securities production realtime endpoint.
	WebSocketURLReal = "wss://openapi.ls-sec.co.kr:9443/websocket"
	// WebSocketURLSandbox is the LS Securities mock realtime endpoint.
	WebSocketURLSandbox = "wss://openapi.ls-sec.co.kr:29443/websocket"
)

const (
	PathOAuthToken      = "/oauth2/token" //nolint:gosec // G101: URL path constant, not a credential
	PathStockMarket     = "/stock/market-data"
	PathStockChart      = "/stock/chart"
	PathStockAccount    = "/stock/accno"
	PathStockOrder      = "/stock/order"
	PathStockMisc       = "/stock/etc"
	PathStockItemSearch = "/stock/item-search"
	PathStockInvestInfo = "/stock/investinfo"

	PathOverseasStockMarket = "/overseas-stock/market-data"
	PathOverseasStockChart  = "/overseas-stock/chart"
)

const (
	TRStockQuote          = "t1102"
	TRStockChart          = "t8410"
	TRStockBalance        = "t0424"
	TRStockOrder          = "CSPAT00601"
	TRStockMaster         = "t8436"
	TRForeignIndexHistory = "t3518"
	TRForeignIndexQuote   = "t3521"
	TRRealtimeKOSPI       = "S3_"
	TRRealtimeKOSDAQ      = "K3_"
	TRRealtimeUnified     = "US3"
	TRRealtimeNXT         = "NS3"

	TROverseasStockQuote      = "g3101"
	TROverseasStockInstrument = "g3104"
	TROverseasStockMaster     = "g3190"
	TROverseasStockChart      = "g3204"
	TRRealtimeOverseasTrade   = "GSC"
	TRRealtimeOverseasQuote   = "GSH"
)
