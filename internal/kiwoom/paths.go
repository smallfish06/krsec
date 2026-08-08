package kiwoom

// Kiwoom REST path prefixes.
const (
	PathPrefixAPI      = "/api"
	PathPrefixAPISlash = "/api/"
)

// Kiwoom endpoint paths.
const (
	PathStockInfo   = "/api/dostk/stkinfo"
	PathMarketCond  = "/api/dostk/mrkcond"
	PathForeignInst = "/api/dostk/frgnistt"
	PathRankingInfo = "/api/dostk/rkinfo"
	PathSector      = "/api/dostk/sect"
	PathELW         = "/api/dostk/elw"
	PathETF         = "/api/dostk/etf"
	PathTheme       = "/api/dostk/thme"
	PathSLB         = "/api/dostk/slb"
	PathShortSell   = "/api/dostk/shsa"
	PathAccount     = "/api/dostk/acnt"
	PathChart       = "/api/dostk/chart"
	PathOrder       = "/api/dostk/ordr"
	PathCreditOrder = "/api/dostk/crdordr" //nolint:gosec // G101: URL path constant, not a credential
	PathWebSocket   = "/api/dostk/websocket"
)
