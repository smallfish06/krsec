package broker

import "time"

// Credentials holds broker authentication credentials
type Credentials struct {
	AppKey    string `json:"app_key"`
	AppSecret string `json:"app_secret"`
}

// Token represents an authentication token
type Token struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Quote represents a stock quote
type Quote struct {
	Symbol      string    `json:"symbol"`
	Market      string    `json:"market"`
	Price       float64   `json:"price"`
	Open        float64   `json:"open"`
	High        float64   `json:"high"`
	Low         float64   `json:"low"`
	Close       float64   `json:"close"`
	PrevClose   float64   `json:"prev_close,omitempty"`
	Change      float64   `json:"change,omitempty"`
	ChangeRate  float64   `json:"change_rate,omitempty"`
	Volume      int64     `json:"volume"`
	Turnover    float64   `json:"turnover,omitempty"`
	UpperLimit  float64   `json:"upper_limit,omitempty"`
	LowerLimit  float64   `json:"lower_limit,omitempty"`
	BidPrice    float64   `json:"bid_price,omitempty"`
	AskPrice    float64   `json:"ask_price,omitempty"`
	BidSize     int64     `json:"bid_size,omitempty"`
	AskSize     int64     `json:"ask_size,omitempty"`
	MarketState string    `json:"market_state,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// OHLCVOpts options for OHLCV data
type OHLCVOpts struct {
	Interval string // "1d", "1h", "5m" etc.
	From     time.Time
	To       time.Time
	Limit    int
}

// OHLCV represents candlestick data
type OHLCV struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    int64     `json:"volume"`
}

// Balance represents account balance
type Balance struct {
	AccountID        string  `json:"account_id"`
	Cash             float64 `json:"cash"`
	TotalAssets      float64 `json:"total_assets"`
	BuyingPower      float64 `json:"buying_power"`
	WithdrawableCash float64 `json:"withdrawable_cash,omitempty"`
	ReceivableAmount float64 `json:"receivable_amount,omitempty"`
	ProfitLoss       float64 `json:"profit_loss"`
	ProfitLossPct    float64 `json:"profit_loss_pct"`
	PositionCost     float64 `json:"position_cost,omitempty"`
	PositionValue    float64 `json:"position_value,omitempty"`
	SettlementT1     float64 `json:"settlement_t1,omitempty"`
	Unsettled        float64 `json:"unsettled,omitempty"`
	LoanBalance      float64 `json:"loan_balance,omitempty"`

	CashByCurrency          map[string]float64 `json:"cash_by_currency,omitempty"`
	TotalAssetsByCurrency   map[string]float64 `json:"total_assets_by_currency,omitempty"`
	BuyingPowerByCurrency   map[string]float64 `json:"buying_power_by_currency,omitempty"`
	ProfitLossByCurrency    map[string]float64 `json:"profit_loss_by_currency,omitempty"`
	PositionCostByCurrency  map[string]float64 `json:"position_cost_by_currency,omitempty"`
	PositionValueByCurrency map[string]float64 `json:"position_value_by_currency,omitempty"`
}

// AssetType represents the type of asset
type AssetType string

const (
	AssetStock    AssetType = "stock"
	AssetBond     AssetType = "bond"
	AssetFund     AssetType = "fund"
	AssetCash     AssetType = "cash"
	AssetOverseas AssetType = "overseas"
)

// Position represents a stock position
type Position struct {
	Symbol              string    `json:"symbol"`
	Name                string    `json:"name"`
	Market              string    `json:"market"`
	MarketCode          string    `json:"market_code,omitempty"`
	AssetType           AssetType `json:"asset_type"`
	Quantity            int64     `json:"quantity"`
	QuantityDecimal     string    `json:"quantity_decimal,omitempty"`
	OrderableQty        int64     `json:"orderable_qty,omitempty"`
	OrderableQtyDecimal string    `json:"orderable_qty_decimal,omitempty"`
	UnsettledQty        int64     `json:"unsettled_qty,omitempty"`
	TodayBuyQty         int64     `json:"today_buy_qty,omitempty"`
	TodaySellQty        int64     `json:"today_sell_qty,omitempty"`
	AvgPrice            float64   `json:"avg_price"`
	CurrentPrice        float64   `json:"current_price"`
	PurchaseValue       float64   `json:"purchase_value,omitempty"`
	MarketValue         float64   `json:"market_value,omitempty"`
	ProfitLoss          float64   `json:"profit_loss"`
	ProfitLossPct       float64   `json:"profit_loss_pct"`
	WeightPct           float64   `json:"weight_pct,omitempty"`
	LoanDate            string    `json:"loan_date,omitempty"`
}

// Instrument represents normalized instrument metadata.
type Instrument struct {
	Symbol          string    `json:"symbol"`
	Market          string    `json:"market"`
	ISIN            string    `json:"isin,omitempty"`
	Name            string    `json:"name"`
	NameEn          string    `json:"name_en,omitempty"`
	ShortName       string    `json:"short_name,omitempty"`
	Exchange        string    `json:"exchange,omitempty"`
	Currency        string    `json:"currency,omitempty"`
	Country         string    `json:"country,omitempty"`
	AssetType       AssetType `json:"asset_type,omitempty"`
	ProductType     string    `json:"product_type,omitempty"`
	ProductTypeCode string    `json:"product_type_code,omitempty"`
	SecurityGroup   string    `json:"security_group,omitempty"`
	Sector          string    `json:"sector,omitempty"`
	ListedShares    int64     `json:"listed_shares,omitempty"`
	IsListed        bool      `json:"is_listed"`
	IsSuspended     bool      `json:"is_suspended"`
	ListingDate     string    `json:"listing_date,omitempty"`
	DelistingDate   string    `json:"delisting_date,omitempty"`
}

// AccountSummary represents aggregated balance across multiple accounts
type AccountSummary struct {
	TotalAssets     float64   `json:"total_assets"`
	TotalCash       float64   `json:"total_cash"`
	TotalProfitLoss float64   `json:"total_profit_loss"`
	Accounts        []Balance `json:"accounts"`
}

// AccountInfo represents basic account information
type AccountInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Broker string `json:"broker"`
}

// OrderSide represents buy or sell
type OrderSide string

const (
	OrderSideBuy  OrderSide = "buy"
	OrderSideSell OrderSide = "sell"
)

// OrderType represents order type
type OrderType string

const (
	OrderTypeLimit  OrderType = "limit"
	OrderTypeMarket OrderType = "market"
)

// OrderStatus represents order status
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusFilled    OrderStatus = "filled"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusRejected  OrderStatus = "rejected"
)

// OrderRequest represents a new order request
type OrderRequest struct {
	AccountID             string    `json:"account_id"`
	ClientOrderID         string    `json:"client_order_id,omitempty"`
	Symbol                string    `json:"symbol"`
	Market                string    `json:"market"`
	Side                  OrderSide `json:"side"`
	Type                  OrderType `json:"type"`
	TimeInForce           string    `json:"time_in_force,omitempty"`
	Quantity              int64     `json:"quantity"`
	QuantityDecimal       string    `json:"quantity_decimal,omitempty"`
	Price                 float64   `json:"price,omitempty"`
	OrderAmount           float64   `json:"order_amount,omitempty"`
	OrderAmountDecimal    string    `json:"order_amount_decimal,omitempty"`
	ConfirmHighValueOrder bool      `json:"confirm_high_value_order,omitempty"`
}

// ModifyOrderRequest represents an order modification request
type ModifyOrderRequest struct {
	Type                  OrderType `json:"type,omitempty"`
	Quantity              int64     `json:"quantity,omitempty"`
	QuantityDecimal       string    `json:"quantity_decimal,omitempty"`
	Price                 float64   `json:"price,omitempty"`
	ConfirmHighValueOrder bool      `json:"confirm_high_value_order,omitempty"`
}

// OrderResult represents the result of an order operation
type OrderResult struct {
	OrderID        string      `json:"order_id"`
	Status         OrderStatus `json:"status"`
	FilledQuantity int64       `json:"filled_quantity,omitempty"`
	RemainingQty   int64       `json:"remaining_quantity,omitempty"`
	AvgFilledPrice float64     `json:"avg_filled_price,omitempty"`
	RejectedReason string      `json:"rejected_reason,omitempty"`
	Message        string      `json:"message,omitempty"`
	Timestamp      time.Time   `json:"timestamp"`
}

// OrderFill represents normalized fill execution data.
type OrderFill struct {
	OrderID   string    `json:"order_id"`
	Symbol    string    `json:"symbol,omitempty"`
	Market    string    `json:"market,omitempty"`
	Side      string    `json:"side,omitempty"`
	Quantity  int64     `json:"quantity"`
	Price     float64   `json:"price"`
	Amount    float64   `json:"amount,omitempty"`
	Currency  string    `json:"currency,omitempty"`
	FilledAt  time.Time `json:"filled_at,omitempty"`
	RawStatus string    `json:"raw_status,omitempty"`
}
