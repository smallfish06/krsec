package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/smallfish06/krsec/internal/ls"
	"github.com/smallfish06/krsec/pkg/broker"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

// Adapter adapts LS Securities OpenAPI into broker.Broker.
type Adapter struct {
	client    *ls.Client
	accountID string
	sandbox   bool
	logger    *slog.Logger
}

// NewAdapterWithOptions creates an LS adapter with injectable internals.
func NewAdapterWithOptions(
	sandbox bool,
	accountID string,
	tokenManager tokencache.Manager,
	macAddress string,
	logger *slog.Logger,
) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	client := ls.NewClientWithTokenManager(sandbox, tokenManager)
	client.SetLogger(logger)
	client.SetMACAddress(macAddress)
	return &Adapter{
		client:    client,
		accountID: strings.TrimSpace(accountID),
		sandbox:   sandbox,
		logger:    logger,
	}
}

// Client exposes the underlying LS client for advanced users and tests.
func (a *Adapter) Client() *ls.Client {
	return a.client
}

// Name returns broker name.
func (a *Adapter) Name() string {
	return broker.NameLS
}

// Authenticate authenticates with LS.
func (a *Adapter) Authenticate(ctx context.Context, creds broker.Credentials) (*broker.Token, error) {
	return a.client.Authenticate(ctx, creds)
}

// ConnectRealtime opens an LS realtime WebSocket connection.
func (a *Adapter) ConnectRealtime(ctx context.Context) (*ls.RealtimeConn, error) {
	return a.client.ConnectRealtime(ctx)
}

// BuildTradeSubscriptions returns KOSPI/KOSDAQ trade subscriptions from LS stock master.
func (a *Adapter) BuildTradeSubscriptions(ctx context.Context) ([]ls.RealtimeSubscription, error) {
	return a.client.BuildTradeSubscriptions(ctx)
}

// BuildOverseasTradeSubscriptions returns overseas trade subscriptions from LS overseas stock master.
func (a *Adapter) BuildOverseasTradeSubscriptions(ctx context.Context, market string, maxRows int) ([]ls.RealtimeSubscription, error) {
	exchangeGroup, ok := lsOverseasExchangeGroup(market)
	if !ok {
		return nil, broker.ErrInvalidMarket
	}
	return a.client.BuildOverseasTradeSubscriptions(ctx, "US", exchangeGroup, maxRows)
}

// GetQuote retrieves a domestic or supported overseas stock quote.
func (a *Adapter) GetQuote(ctx context.Context, market, symbol string) (*broker.Quote, error) {
	if exchange, ok := lsOverseasExchangeCode(market); ok {
		return a.getOverseasQuote(ctx, market, symbol, exchange)
	}

	exchange := lsExchangeCode(market)
	if exchange == "" {
		return nil, broker.ErrInvalidMarket
	}
	row, err := a.client.InquirePrice(ctx, symbol, exchange)
	if err != nil {
		return nil, err
	}
	price := anyFloat(row["price"])
	prevClose := anyFloat(row["recprice"])
	if prevClose == 0 && price != 0 {
		prevClose = price - anyFloat(row["change"])
	}
	symbolOut := normalizeSymbol(anyString(row["shcode"]))
	if symbolOut == "" {
		symbolOut = normalizeSymbol(symbol)
	}
	return &broker.Quote{
		Symbol:     symbolOut,
		Market:     normalizeOutputMarket(market, row),
		Price:      price,
		Open:       anyFloat(row["open"]),
		High:       anyFloat(row["high"]),
		Low:        anyFloat(row["low"]),
		Close:      price,
		PrevClose:  prevClose,
		Change:     anyFloat(row["change"]),
		ChangeRate: anyFloat(row["diff"]),
		Volume:     anyInt64(row["volume"]),
		Turnover:   anyFloat(row["value"]),
		UpperLimit: anyFloat(row["uplmtprice"]),
		LowerLimit: anyFloat(row["dnlmtprice"]),
		Timestamp:  time.Now(),
	}, nil
}

func (a *Adapter) getOverseasQuote(ctx context.Context, market, symbol, exchange string) (*broker.Quote, error) {
	row, err := a.client.InquireOverseasPrice(ctx, symbol, exchange)
	if err != nil {
		return nil, err
	}
	price := anyFloat(row["price"])
	change := lsSignedDiff(anyFloat(row["diff"]), anyString(row["sign"]))
	prevClose := 0.0
	if price != 0 || change != 0 {
		prevClose = price - change
	}
	symbolOut := normalizeOverseasSymbol(anyString(row["symbol"]))
	if symbolOut == "" {
		symbolOut = normalizeOverseasSymbol(symbol)
	}
	return &broker.Quote{
		Symbol:      symbolOut,
		Market:      normalizeOverseasOutputMarket(market, row),
		Price:       price,
		Open:        anyFloat(row["open"]),
		High:        anyFloat(row["high"]),
		Low:         anyFloat(row["low"]),
		Close:       price,
		PrevClose:   prevClose,
		Change:      change,
		ChangeRate:  anyFloat(row["rate"]),
		Volume:      anyInt64(row["volume"]),
		Turnover:    anyFloat(row["amount"]),
		UpperLimit:  anyFloat(row["uplimit"]),
		LowerLimit:  anyFloat(row["dnlimit"]),
		MarketState: anyString(row["suspend"]),
		Timestamp:   time.Now(),
	}, nil
}

// GetOHLCV retrieves daily/weekly/monthly candles.
func (a *Adapter) GetOHLCV(ctx context.Context, market, symbol string, opts broker.OHLCVOpts) ([]broker.OHLCV, error) {
	if exchange, ok := lsOverseasExchangeCode(market); ok {
		return a.getOverseasOHLCV(ctx, symbol, exchange, opts)
	}

	if lsExchangeCode(market) == "" {
		return nil, broker.ErrInvalidMarket
	}
	rows, err := a.client.InquireChart(ctx, symbol, opts.Interval, opts.From, opts.To, opts.Limit)
	if err != nil {
		return nil, err
	}
	out := make([]broker.OHLCV, 0, len(rows))
	for _, row := range rows {
		t, ok := parseYYYYMMDD(anyString(row["date"]))
		if !ok {
			continue
		}
		item := broker.OHLCV{
			Timestamp: t,
			Open:      anyFloat(row["open"]),
			High:      anyFloat(row["high"]),
			Low:       anyFloat(row["low"]),
			Close:     anyFloat(row["close"]),
			Volume:    anyInt64(row["jdiff_vol"]),
		}
		if !opts.From.IsZero() && item.Timestamp.Before(startOfDay(opts.From)) {
			continue
		}
		if !opts.To.IsZero() && item.Timestamp.After(endOfDay(opts.To)) {
			continue
		}
		out = append(out, item)
	}
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

func (a *Adapter) getOverseasOHLCV(ctx context.Context, symbol, exchange string, opts broker.OHLCVOpts) ([]broker.OHLCV, error) {
	rows, err := a.client.InquireOverseasChart(ctx, symbol, exchange, opts.Interval, opts.From, opts.To, opts.Limit)
	if err != nil {
		return nil, err
	}
	out := make([]broker.OHLCV, 0, len(rows))
	for _, row := range rows {
		t, ok := parseYYYYMMDD(anyString(row["date"]))
		if !ok {
			continue
		}
		item := broker.OHLCV{
			Timestamp: t,
			Open:      anyFloat(row["open"]),
			High:      anyFloat(row["high"]),
			Low:       anyFloat(row["low"]),
			Close:     anyFloat(row["close"]),
			Volume:    anyInt64(row["volume"]),
		}
		if !opts.From.IsZero() && item.Timestamp.Before(startOfDay(opts.From)) {
			continue
		}
		if !opts.To.IsZero() && item.Timestamp.After(endOfDay(opts.To)) {
			continue
		}
		out = append(out, item)
	}
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

// GetBalance retrieves LS stock account balance summary.
func (a *Adapter) GetBalance(ctx context.Context, accountID string) (*broker.Balance, error) {
	if strings.TrimSpace(accountID) == "" {
		accountID = a.accountID
	}
	summary, _, err := a.client.InquireBalance(ctx)
	if err != nil {
		return nil, err
	}
	totalAssets := anyFloat(summary["tappamt"])
	if totalAssets == 0 {
		totalAssets = anyFloat(summary["sunamt"])
	}
	return &broker.Balance{
		AccountID:     strings.TrimSpace(accountID),
		Cash:          anyFloat(summary["sunamt"]),
		TotalAssets:   totalAssets,
		BuyingPower:   anyFloat(summary["sunamt"]),
		ProfitLoss:    anyFloat(summary["tdtsunik"]),
		PositionCost:  anyFloat(summary["mamt"]),
		PositionValue: anyFloat(summary["tappamt"]),
	}, nil
}

// GetPositions retrieves LS account positions.
func (a *Adapter) GetPositions(ctx context.Context, _ string) ([]broker.Position, error) {
	_, rows, err := a.client.InquireBalance(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]broker.Position, 0, len(rows))
	for _, row := range rows {
		symbol := normalizeSymbol(anyString(row["expcode"]))
		if symbol == "" || strings.EqualFold(symbol, "CMARP") {
			continue
		}
		qty := anyInt64(row["janqty"])
		if qty == 0 {
			continue
		}
		position := broker.Position{
			Symbol:        symbol,
			Name:          anyString(row["hname"]),
			Market:        normalizePositionMarket(anyString(row["marketgb"])),
			AssetType:     broker.AssetStock,
			Quantity:      qty,
			OrderableQty:  anyInt64(row["mdposqt"]),
			AvgPrice:      anyFloat(row["pamt"]),
			CurrentPrice:  anyFloat(row["price"]),
			PurchaseValue: anyFloat(row["mamt"]),
			MarketValue:   anyFloat(row["appamt"]),
			ProfitLoss:    anyFloat(row["dtsunik"]),
			ProfitLossPct: anyFloat(row["sunikrt"]),
			WeightPct:     anyFloat(row["janrt"]),
			LoanDate:      anyString(row["loandt"]),
		}
		out = append(out, position)
	}
	return out, nil
}

// PlaceOrder places a regular cash stock order.
func (a *Adapter) PlaceOrder(ctx context.Context, req broker.OrderRequest) (*broker.OrderResult, error) {
	symbol := normalizeOrderSymbol(req.Symbol, a.sandbox)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	side, err := lsOrderSide(req.Side)
	if err != nil {
		return nil, err
	}
	orderType, err := lsOrderType(req.Type)
	if err != nil {
		return nil, err
	}
	if req.Quantity <= 0 {
		return nil, fmt.Errorf("%w: quantity must be greater than 0", broker.ErrInvalidOrderRequest)
	}
	if req.Type == broker.OrderTypeLimit && req.Price <= 0 {
		return nil, fmt.Errorf("%w: limit price must be greater than 0", broker.ErrInvalidOrderRequest)
	}

	resp, err := a.client.CallEndpoint(ctx, "POST", ls.PathStockOrder, ls.TRStockOrder, map[string]interface{}{
		"CSPAT00601InBlock1": map[string]interface{}{
			"IsuNo":         symbol,
			"OrdQty":        req.Quantity,
			"OrdPrc":        req.Price,
			"BnsTpCode":     side,
			"OrdprcPtnCode": orderType,
			"MgntrnCode":    "000",
			"LoanDt":        "",
			"OrdCndiTpCode": "0",
			"MbrNo":         lsMarketMember(req.Market),
		},
	})
	if err != nil {
		return nil, err
	}
	block, _ := mapValue(resp, "CSPAT00601OutBlock2")
	orderID := anyString(block["OrdNo"])
	if orderID == "" {
		return nil, fmt.Errorf("%w: LS order response missing OrdNo", broker.ErrServerError)
	}
	return &broker.OrderResult{
		OrderID:      orderID,
		Status:       broker.OrderStatusPending,
		RemainingQty: req.Quantity,
		Message:      anyString(resp["rsp_msg"]),
		Timestamp:    time.Now(),
	}, nil
}

// CancelOrder requires original symbol and quantity, which the common API does
// not carry yet for LS order cancellation.
func (a *Adapter) CancelOrder(context.Context, string) error {
	return fmt.Errorf("%w: LS cancel requires original symbol and quantity context", broker.ErrNotSupported)
}

// ModifyOrder requires original symbol and order type context, which the common API does not carry yet.
func (a *Adapter) ModifyOrder(context.Context, string, broker.ModifyOrderRequest) (*broker.OrderResult, error) {
	return nil, fmt.Errorf("%w: LS modify requires original symbol and order context", broker.ErrNotSupported)
}

// GetOrder is not implemented until LS order history is mapped into the common contract.
func (a *Adapter) GetOrder(context.Context, string) (*broker.OrderResult, error) {
	return nil, fmt.Errorf("%w: LS order lookup is not implemented", broker.ErrNotSupported)
}

// GetOrderFills is not implemented until LS fill history is mapped into the common contract.
func (a *Adapter) GetOrderFills(context.Context, string) ([]broker.OrderFill, error) {
	return nil, fmt.Errorf("%w: LS order fills lookup is not implemented", broker.ErrNotSupported)
}

// GetInstrument returns stock master metadata.
func (a *Adapter) GetInstrument(ctx context.Context, market, symbol string) (*broker.Instrument, error) {
	if exchange, ok := lsOverseasExchangeCode(market); ok {
		return a.getOverseasInstrument(ctx, market, symbol, exchange)
	}

	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	rows, err := a.client.ListStockMaster(ctx, "0")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.Symbol != symbol {
			continue
		}
		return &broker.Instrument{
			Symbol:    row.Symbol,
			Market:    marketFromMasterCode(row.MarketCode),
			Name:      row.Name,
			Exchange:  "KRX",
			Currency:  "KRW",
			Country:   "KR",
			AssetType: broker.AssetStock,
			IsListed:  true,
		}, nil
	}
	return nil, broker.ErrInstrumentNotFound
}

func (a *Adapter) getOverseasInstrument(ctx context.Context, market, symbol, exchange string) (*broker.Instrument, error) {
	symbol = normalizeOverseasSymbol(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	row, err := a.client.InquireOverseasInstrument(ctx, symbol, exchange)
	if err != nil {
		return nil, err
	}
	symbolOut := normalizeOverseasSymbol(anyString(row["symbol"]))
	if symbolOut == "" {
		symbolOut = symbol
	}
	name := anyString(row["korname"])
	if name == "" {
		name = anyString(row["engname"])
	}
	if name == "" {
		name = symbolOut
	}
	productType := anyString(row["instname"])
	assetType := broker.AssetOverseas
	if strings.Contains(strings.ToUpper(productType), "ETF") {
		assetType = broker.AssetFund
	}
	return &broker.Instrument{
		Symbol:       symbolOut,
		Market:       normalizeOverseasOutputMarket(market, row),
		Name:         name,
		NameEn:       anyString(row["engname"]),
		Exchange:     normalizeOverseasExchangeName(market, row),
		Currency:     strings.ToUpper(anyString(row["currency"])),
		Country:      "US",
		AssetType:    assetType,
		ProductType:  productType,
		Sector:       anyString(row["induname"]),
		ListedShares: anyInt64(row["share"]),
		IsListed:     true,
		IsSuspended:  strings.EqualFold(anyString(row["suspend"]), "Y"),
	}, nil
}
