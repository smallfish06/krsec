package kis

// KIS REST endpoint path constants.
const (
	PathPrefixUAPI      = "/uapi"
	PathPrefixUAPISlash = "/uapi/"
)

const (
	PathDomesticStockInquirePrice               = "/uapi/domestic-stock/v1/quotations/inquire-price"
	PathDomesticStockInquireDailyPrice          = "/uapi/domestic-stock/v1/quotations/inquire-daily-price"
	PathDomesticStockInquireDailyItemChartPrice = "/uapi/domestic-stock/v1/quotations/inquire-daily-itemchartprice"
	PathDomesticStockInquireAskingPriceExpCcn   = "/uapi/domestic-stock/v1/quotations/inquire-asking-price-exp-ccn"
	PathDomesticStockInquireCcnl                = "/uapi/domestic-stock/v1/quotations/inquire-ccnl"
	PathDomesticStockInquireTimeItemConclusion  = "/uapi/domestic-stock/v1/quotations/inquire-time-itemconclusion"
	PathDomesticStockInquireMember              = "/uapi/domestic-stock/v1/quotations/inquire-member"
	PathDomesticStockInquireIndexPrice          = "/uapi/domestic-stock/v1/quotations/inquire-index-price"
	PathDomesticStockInquireIndexDailyPrice     = "/uapi/domestic-stock/v1/quotations/inquire-index-daily-price"
	PathDomesticStockInquireDailyIndexChart     = "/uapi/domestic-stock/v1/quotations/inquire-daily-indexchartprice"
	PathDomesticStockSearchStockInfo            = "/uapi/domestic-stock/v1/quotations/search-stock-info"
	PathDomesticStockSearchInfo                 = "/uapi/domestic-stock/v1/quotations/search-info"
	PathDomesticStockVolumeRank                 = "/uapi/domestic-stock/v1/quotations/volume-rank"
	PathDomesticStockRankingMarketCap           = "/uapi/domestic-stock/v1/ranking/market-cap"
	PathDomesticStockRankingFluctuation         = "/uapi/domestic-stock/v1/ranking/fluctuation"
	PathDomesticStockDividend                   = "/uapi/domestic-stock/v1/ksdinfo/dividend"
)

const (
	PathDomesticStockTradingInquireBalance           = "/uapi/domestic-stock/v1/trading/inquire-balance"
	PathDomesticStockTradingInquirePsblRvseCncl      = "/uapi/domestic-stock/v1/trading/inquire-psbl-rvsecncl"
	PathDomesticStockTradingInquireDailyCcld         = "/uapi/domestic-stock/v1/trading/inquire-daily-ccld"
	PathDomesticStockTradingInquirePsblOrder         = "/uapi/domestic-stock/v1/trading/inquire-psbl-order"
	PathDomesticStockTradingInquirePeriodTradeProfit = "/uapi/domestic-stock/v1/trading/inquire-period-trade-profit"
	PathDomesticStockTradingOrderCash                = "/uapi/domestic-stock/v1/trading/order-cash"
	PathDomesticStockTradingOrderRvseCncl            = "/uapi/domestic-stock/v1/trading/order-rvsecncl"
)

const (
	PathOverseasPricePrice                  = "/uapi/overseas-price/v1/quotations/price"
	PathOverseasPriceInquireDailyChartPrice = "/uapi/overseas-price/v1/quotations/inquire-daily-chartprice"
	PathOverseasPriceDailyPrice             = "/uapi/overseas-price/v1/quotations/dailyprice"
	PathOverseasPricePriceDetail            = "/uapi/overseas-price/v1/quotations/price-detail"
	PathOverseasPriceInquireCcnl            = "/uapi/overseas-price/v1/quotations/inquire-ccnl"
	PathOverseasPriceInquireTimeItemChart   = "/uapi/overseas-price/v1/quotations/inquire-time-itemchartprice"
	PathOverseasPriceSearchInfo             = "/uapi/overseas-price/v1/quotations/search-info"
	PathOverseasStockRankingUpdownRate      = "/uapi/overseas-stock/v1/ranking/updown-rate"
	PathOverseasStockTradingInquireBalance  = "/uapi/overseas-stock/v1/trading/inquire-balance"
	PathOverseasStockTradingInquirePsAmount = "/uapi/overseas-stock/v1/trading/inquire-psamount"
	PathOverseasStockTradingInquireCcnl     = "/uapi/overseas-stock/v1/trading/inquire-ccnl"
	PathOverseasStockTradingOrder           = "/uapi/overseas-stock/v1/trading/order"
	PathOverseasStockTradingOrderRvseCncl   = "/uapi/overseas-stock/v1/trading/order-rvsecncl"
)

const (
	PathDomesticBondInquirePrice      = "/uapi/domestic-bond/v1/quotations/inquire-price"
	PathDomesticBondInquireDailyPrice = "/uapi/domestic-bond/v1/quotations/inquire-daily-price"
	PathDomesticBondSearchBondInfo    = "/uapi/domestic-bond/v1/quotations/search-bond-info"
	PathDomesticBondAvgUnit           = "/uapi/domestic-bond/v1/quotations/avg-unit"
	PathDomesticBondInquireBalance    = "/uapi/domestic-bond/v1/trading/inquire-balance"
	PathETFETNComponentStockPrice     = "/uapi/etfetn/v1/quotations/inquire-component-stock-price"
)
