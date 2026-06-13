package toss

type apiEnvelope[T any] struct {
	Result T `json:"result"`
}

type Account struct {
	AccountNo   string `json:"accountNo"`
	AccountSeq  int64  `json:"accountSeq"`
	AccountType string `json:"accountType"`
}

type PriceResponse struct {
	Symbol    string  `json:"symbol"`
	Timestamp *string `json:"timestamp"`
	LastPrice string  `json:"lastPrice"`
	Currency  string  `json:"currency"`
}

type CandlePageResponse struct {
	Candles    []Candle `json:"candles"`
	NextBefore *string  `json:"nextBefore"`
}

type Candle struct {
	Timestamp  string `json:"timestamp"`
	OpenPrice  string `json:"openPrice"`
	HighPrice  string `json:"highPrice"`
	LowPrice   string `json:"lowPrice"`
	ClosePrice string `json:"closePrice"`
	Volume     string `json:"volume"`
	Currency   string `json:"currency"`
}

type StockInfo struct {
	Symbol             string          `json:"symbol"`
	Name               string          `json:"name"`
	EnglishName        string          `json:"englishName"`
	ISINCode           string          `json:"isinCode"`
	Market             string          `json:"market"`
	SecurityType       string          `json:"securityType"`
	IsCommonShare      bool            `json:"isCommonShare"`
	Status             string          `json:"status"`
	Currency           string          `json:"currency"`
	ListDate           *string         `json:"listDate"`
	DelistDate         *string         `json:"delistDate"`
	SharesOutstanding  string          `json:"sharesOutstanding"`
	LeverageFactor     *string         `json:"leverageFactor"`
	KoreanMarketDetail *KrMarketDetail `json:"koreanMarketDetail"`
}

type KrMarketDetail struct {
	LiquidationTrading  bool  `json:"liquidationTrading"`
	NXTSupported        bool  `json:"nxtSupported"`
	KRXTradingSuspended bool  `json:"krxTradingSuspended"`
	NXTTradingSuspended *bool `json:"nxtTradingSuspended"`
}

type HoldingsOverview struct {
	TotalPurchaseAmount MultiCurrencyAmount     `json:"totalPurchaseAmount"`
	MarketValue         OverviewMarketValue     `json:"marketValue"`
	ProfitLoss          OverviewProfitLoss      `json:"profitLoss"`
	DailyProfitLoss     OverviewDailyProfitLoss `json:"dailyProfitLoss"`
	Items               []HoldingsItem          `json:"items"`
}

type MultiCurrencyAmount struct {
	KRW string  `json:"krw"`
	USD *string `json:"usd"`
}

type OverviewMarketValue struct {
	Amount          MultiCurrencyAmount `json:"amount"`
	AmountAfterCost MultiCurrencyAmount `json:"amountAfterCost"`
}

type OverviewProfitLoss struct {
	Amount          MultiCurrencyAmount `json:"amount"`
	AmountAfterCost MultiCurrencyAmount `json:"amountAfterCost"`
	Rate            string              `json:"rate"`
	RateAfterCost   string              `json:"rateAfterCost"`
}

type OverviewDailyProfitLoss struct {
	Amount MultiCurrencyAmount `json:"amount"`
	Rate   string              `json:"rate"`
}

type DailyProfitLoss struct {
	Amount string `json:"amount"`
	Rate   string `json:"rate"`
}

type HoldingsItem struct {
	Symbol               string             `json:"symbol"`
	Name                 string             `json:"name"`
	MarketCountry        string             `json:"marketCountry"`
	Currency             string             `json:"currency"`
	Quantity             string             `json:"quantity"`
	LastPrice            string             `json:"lastPrice"`
	AveragePurchasePrice string             `json:"averagePurchasePrice"`
	MarketValue          HoldingMarketValue `json:"marketValue"`
	ProfitLoss           HoldingProfitLoss  `json:"profitLoss"`
	DailyProfitLoss      DailyProfitLoss    `json:"dailyProfitLoss"`
	Cost                 Cost               `json:"cost"`
}

type HoldingMarketValue struct {
	PurchaseAmount  string `json:"purchaseAmount"`
	Amount          string `json:"amount"`
	AmountAfterCost string `json:"amountAfterCost"`
}

type HoldingProfitLoss struct {
	Amount          string `json:"amount"`
	AmountAfterCost string `json:"amountAfterCost"`
	Rate            string `json:"rate"`
	RateAfterCost   string `json:"rateAfterCost"`
}

type Cost struct {
	Commission *string `json:"commission"`
	Tax        *string `json:"tax"`
}

type BuyingPowerResponse struct {
	Currency        string `json:"currency"`
	CashBuyingPower string `json:"cashBuyingPower"`
}

type SellableQuantityResponse struct {
	SellableQuantity string `json:"sellableQuantity"`
}

type Commission struct {
	MarketCountry  string  `json:"marketCountry"`
	CommissionRate string  `json:"commissionRate"`
	StartDate      *string `json:"startDate"`
	EndDate        *string `json:"endDate"`
}

type OrderResponse struct {
	OrderID       string  `json:"orderId"`
	ClientOrderID *string `json:"clientOrderId"`
}

type OrderOperationResponse struct {
	OrderID string `json:"orderId"`
}

type PaginatedOrderResponse struct {
	Orders     []Order `json:"orders"`
	NextCursor *string `json:"nextCursor"`
	HasNext    bool    `json:"hasNext"`
}

type Order struct {
	OrderID     string         `json:"orderId"`
	Symbol      string         `json:"symbol"`
	Side        string         `json:"side"`
	OrderType   string         `json:"orderType"`
	TimeInForce string         `json:"timeInForce"`
	Status      string         `json:"status"`
	Price       *string        `json:"price"`
	Quantity    string         `json:"quantity"`
	OrderAmount *string        `json:"orderAmount"`
	Currency    string         `json:"currency"`
	OrderedAt   string         `json:"orderedAt"`
	CanceledAt  *string        `json:"canceledAt"`
	Execution   OrderExecution `json:"execution"`
}

type OrderExecution struct {
	FilledQuantity     string  `json:"filledQuantity"`
	AverageFilledPrice *string `json:"averageFilledPrice"`
	FilledAmount       *string `json:"filledAmount"`
	Commission         *string `json:"commission"`
	Tax                *string `json:"tax"`
	FilledAt           *string `json:"filledAt"`
	SettlementDate     *string `json:"settlementDate"`
}
