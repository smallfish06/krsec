package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/smallfish06/krsec/internal/toss"
	"github.com/smallfish06/krsec/pkg/broker"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

// Adapter adapts Toss Securities Open API into broker.Broker.
type Adapter struct {
	client     *toss.Client
	accountID  string
	accountSeq string
	sandbox    bool
	logger     *slog.Logger
}

// NewAdapterWithOptions creates a Toss adapter with injectable internals.
func NewAdapterWithOptions(
	sandbox bool,
	accountID string,
	accountSeq string,
	tokenManager tokencache.Manager,
	logger *slog.Logger,
) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	client := toss.NewClientWithTokenManager(sandbox, tokenManager)
	client.SetLogger(logger)
	return &Adapter{
		client:     client,
		accountID:  strings.TrimSpace(accountID),
		accountSeq: strings.TrimSpace(accountSeq),
		sandbox:    sandbox,
		logger:     logger,
	}
}

// Client exposes the underlying Toss client for tests and advanced callers.
func (a *Adapter) Client() *toss.Client {
	return a.client
}

// Name returns broker name.
func (a *Adapter) Name() string {
	return broker.NameToss
}

// Authenticate authenticates with Toss.
func (a *Adapter) Authenticate(ctx context.Context, creds broker.Credentials) (*broker.Token, error) {
	return a.client.Authenticate(ctx, creds)
}

// CallEndpoint dispatches a raw Toss Open API endpoint using this adapter's account_seq.
func (a *Adapter) CallEndpoint(ctx context.Context, method, path string, query map[string]interface{}, body interface{}) (interface{}, error) {
	return a.client.CallEndpoint(ctx, method, path, a.accountSeq, query, body)
}

// GetQuote retrieves a normalized stock quote from Toss prices and the latest daily candle.
func (a *Adapter) GetQuote(ctx context.Context, market, symbol string) (*broker.Quote, error) {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	prices, err := a.client.GetPrices(ctx, symbol)
	if err != nil {
		return nil, err
	}
	var price toss.PriceResponse
	for _, item := range prices {
		if strings.EqualFold(strings.TrimSpace(item.Symbol), symbol) {
			price = item
			break
		}
	}
	if price.Symbol == "" && len(prices) > 0 {
		price = prices[0]
	}
	if price.Symbol == "" {
		return nil, broker.ErrInstrumentNotFound
	}

	last := parseDecimal(price.LastPrice)
	ts := parseOptionalTime(price.Timestamp)
	if ts.IsZero() {
		ts = time.Now()
	}
	q := &broker.Quote{
		Symbol:    firstNonEmpty(price.Symbol, symbol),
		Market:    strings.ToUpper(strings.TrimSpace(market)),
		Price:     last,
		Close:     last,
		Timestamp: ts,
	}

	if candles, candleErr := a.client.GetCandles(ctx, symbol, "1d", 1, "", true); candleErr == nil && len(candles.Candles) > 0 {
		c := candles.Candles[0]
		q.Open = parseDecimal(c.OpenPrice)
		q.High = parseDecimal(c.HighPrice)
		q.Low = parseDecimal(c.LowPrice)
		q.Close = parseDecimal(c.ClosePrice)
		q.Volume = parseDecimalInt64(c.Volume)
		if q.Close != 0 || q.Price != 0 {
			q.Change = q.Price - q.Close
			q.PrevClose = q.Close
		}
		if q.Close != 0 && q.Change != 0 {
			q.ChangeRate = q.Change / q.Close * 100
		}
	}

	return q, nil
}

// GetOHLCV retrieves Toss 1d or 1m candles.
func (a *Adapter) GetOHLCV(ctx context.Context, market, symbol string, opts broker.OHLCVOpts) ([]broker.OHLCV, error) {
	_ = market
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	interval, err := tossInterval(opts.Interval)
	if err != nil {
		return nil, err
	}
	count := opts.Limit
	if count <= 0 {
		count = 100
	}
	if count > 200 {
		count = 200
	}
	before := ""
	if !opts.To.IsZero() {
		before = opts.To.Format(time.RFC3339)
	}
	page, err := a.client.GetCandles(ctx, symbol, interval, count, before, true)
	if err != nil {
		return nil, err
	}
	out := make([]broker.OHLCV, 0, len(page.Candles))
	for _, item := range page.Candles {
		t := parseTime(item.Timestamp)
		if t.IsZero() {
			continue
		}
		if !opts.From.IsZero() && t.Before(startOfDay(opts.From)) {
			continue
		}
		if !opts.To.IsZero() && t.After(endOfDay(opts.To)) {
			continue
		}
		out = append(out, broker.OHLCV{
			Timestamp: t,
			Open:      parseDecimal(item.OpenPrice),
			High:      parseDecimal(item.HighPrice),
			Low:       parseDecimal(item.LowPrice),
			Close:     parseDecimal(item.ClosePrice),
			Volume:    parseDecimalInt64(item.Volume),
		})
	}
	return out, nil
}

// GetBalance retrieves a Toss account balance summary from holdings and buying-power APIs.
func (a *Adapter) GetBalance(ctx context.Context, accountID string) (*broker.Balance, error) {
	if strings.TrimSpace(accountID) == "" {
		accountID = a.accountID
	}
	holdings, err := a.client.GetHoldings(ctx, a.accountSeq, "")
	if err != nil {
		return nil, err
	}

	buyingPowerByCurrency := map[string]float64{}
	for _, currency := range []string{"KRW", "USD"} {
		power, powerErr := a.client.GetBuyingPower(ctx, a.accountSeq, currency)
		if powerErr != nil {
			continue
		}
		buyingPowerByCurrency[strings.ToUpper(power.Currency)] = parseDecimal(power.CashBuyingPower)
	}

	cash := buyingPowerByCurrency["KRW"]
	return &broker.Balance{
		AccountID:               strings.TrimSpace(accountID),
		Cash:                    cash,
		TotalAssets:             parseDecimal(holdings.MarketValue.Amount.KRW),
		BuyingPower:             cash,
		WithdrawableCash:        cash,
		ProfitLoss:              parseDecimal(holdings.ProfitLoss.Amount.KRW),
		ProfitLossPct:           parseDecimal(holdings.ProfitLoss.Rate) * 100,
		PositionCost:            parseDecimal(holdings.TotalPurchaseAmount.KRW),
		PositionValue:           parseDecimal(holdings.MarketValue.Amount.KRW),
		CashByCurrency:          cloneCurrencyMap(buyingPowerByCurrency),
		BuyingPowerByCurrency:   cloneCurrencyMap(buyingPowerByCurrency),
		TotalAssetsByCurrency:   multiCurrencyMap(holdings.MarketValue.Amount),
		ProfitLossByCurrency:    multiCurrencyMap(holdings.ProfitLoss.Amount),
		PositionCostByCurrency:  multiCurrencyMap(holdings.TotalPurchaseAmount),
		PositionValueByCurrency: multiCurrencyMap(holdings.MarketValue.Amount),
	}, nil
}

// GetPositions retrieves Toss account positions.
func (a *Adapter) GetPositions(ctx context.Context, _ string) ([]broker.Position, error) {
	holdings, err := a.client.GetHoldings(ctx, a.accountSeq, "")
	if err != nil {
		return nil, err
	}
	out := make([]broker.Position, 0, len(holdings.Items))
	for _, item := range holdings.Items {
		qty := decimalToInt64(item.Quantity)
		if strings.TrimSpace(item.Symbol) == "" || (qty == 0 && parseDecimal(item.Quantity) == 0) {
			continue
		}
		out = append(out, broker.Position{
			Symbol:          item.Symbol,
			Name:            item.Name,
			Market:          marketFromCountry(item.MarketCountry),
			AssetType:       assetTypeFromHolding(item),
			Quantity:        qty,
			QuantityDecimal: item.Quantity,
			AvgPrice:        parseDecimal(item.AveragePurchasePrice),
			CurrentPrice:    parseDecimal(item.LastPrice),
			PurchaseValue:   parseDecimal(item.MarketValue.PurchaseAmount),
			MarketValue:     parseDecimal(item.MarketValue.Amount),
			ProfitLoss:      parseDecimal(item.ProfitLoss.Amount),
			ProfitLossPct:   parseDecimal(item.ProfitLoss.Rate) * 100,
		})
	}
	return out, nil
}

// GetInstrument returns Toss stock metadata.
func (a *Adapter) GetInstrument(ctx context.Context, market, symbol string) (*broker.Instrument, error) {
	_ = market
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	stocks, err := a.client.GetStocks(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if len(stocks) == 0 {
		return nil, broker.ErrInstrumentNotFound
	}
	item := stocks[0]
	return &broker.Instrument{
		Symbol:        firstNonEmpty(item.Symbol, symbol),
		Market:        item.Market,
		ISIN:          item.ISINCode,
		Name:          firstNonEmpty(item.Name, item.EnglishName, symbol),
		NameEn:        item.EnglishName,
		Exchange:      item.Market,
		Currency:      item.Currency,
		Country:       countryFromMarket(item.Market),
		AssetType:     assetTypeFromStock(item),
		ProductType:   item.SecurityType,
		ListedShares:  decimalToInt64(item.SharesOutstanding),
		IsListed:      strings.EqualFold(item.Status, "ACTIVE"),
		IsSuspended:   stockSuspended(item),
		ListingDate:   stringPtrValue(item.ListDate),
		DelistingDate: stringPtrValue(item.DelistDate),
	}, nil
}

// PlaceOrder places a Toss stock order.
func (a *Adapter) PlaceOrder(ctx context.Context, req broker.OrderRequest) (*broker.OrderResult, error) {
	body, err := buildCreateOrderBody(req)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.CreateOrder(ctx, a.accountSeq, body)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resp.OrderID) == "" {
		return nil, fmt.Errorf("%w: Toss order response missing orderId", broker.ErrServerError)
	}
	return &broker.OrderResult{
		OrderID:      resp.OrderID,
		Status:       broker.OrderStatusPending,
		RemainingQty: requestQuantity(req),
		Timestamp:    time.Now(),
	}, nil
}

// CancelOrder cancels a Toss order.
func (a *Adapter) CancelOrder(ctx context.Context, orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return broker.ErrOrderNotFound
	}
	_, err := a.client.CancelOrder(ctx, a.accountSeq, orderID)
	return err
}

// ModifyOrder modifies a Toss order.
func (a *Adapter) ModifyOrder(ctx context.Context, orderID string, req broker.ModifyOrderRequest) (*broker.OrderResult, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, broker.ErrOrderNotFound
	}
	body, err := buildModifyOrderBody(req)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.ModifyOrder(ctx, a.accountSeq, orderID, body)
	if err != nil {
		return nil, err
	}
	newOrderID := firstNonEmpty(resp.OrderID, orderID)
	return &broker.OrderResult{
		OrderID:      newOrderID,
		Status:       broker.OrderStatusPending,
		RemainingQty: requestModifyQuantity(req),
		Timestamp:    time.Now(),
	}, nil
}

// GetOrder returns a Toss order snapshot.
func (a *Adapter) GetOrder(ctx context.Context, orderID string) (*broker.OrderResult, error) {
	order, err := a.client.GetOrder(ctx, a.accountSeq, orderID)
	if err != nil {
		return nil, err
	}
	return orderToResult(order), nil
}

// GetOrderFills returns a normalized aggregate fill for a Toss order.
func (a *Adapter) GetOrderFills(ctx context.Context, orderID string) ([]broker.OrderFill, error) {
	order, err := a.client.GetOrder(ctx, a.accountSeq, orderID)
	if err != nil {
		return nil, err
	}
	filledQty := decimalToInt64(order.Execution.FilledQuantity)
	if filledQty <= 0 {
		return []broker.OrderFill{}, nil
	}
	return []broker.OrderFill{{
		OrderID:   order.OrderID,
		Symbol:    order.Symbol,
		Market:    marketFromCurrency(order.Currency),
		Side:      strings.ToLower(order.Side),
		Quantity:  filledQty,
		Price:     parseOptionalDecimal(order.Execution.AverageFilledPrice),
		Amount:    parseOptionalDecimal(order.Execution.FilledAmount),
		Currency:  order.Currency,
		FilledAt:  parseOptionalStringTime(order.Execution.FilledAt),
		RawStatus: order.Status,
	}}, nil
}

func (a *Adapter) GetOrders(ctx context.Context, query map[string]interface{}) (toss.PaginatedOrderResponse, error) {
	return a.client.GetOrders(ctx, a.accountSeq, query)
}

func (a *Adapter) GetBuyingPower(ctx context.Context, currency string) (toss.BuyingPowerResponse, error) {
	return a.client.GetBuyingPower(ctx, a.accountSeq, currency)
}

func (a *Adapter) GetSellableQuantity(ctx context.Context, symbol string) (toss.SellableQuantityResponse, error) {
	return a.client.GetSellableQuantity(ctx, a.accountSeq, symbol)
}

func (a *Adapter) GetCommissions(ctx context.Context) ([]toss.Commission, error) {
	return a.client.GetCommissions(ctx, a.accountSeq)
}

func buildCreateOrderBody(req broker.OrderRequest) (map[string]interface{}, error) {
	symbol := strings.TrimSpace(req.Symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	side, err := tossOrderSide(req.Side)
	if err != nil {
		return nil, err
	}
	orderType, err := tossOrderType(req.Type)
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"symbol":    symbol,
		"side":      side,
		"orderType": orderType,
	}
	if req.ClientOrderID != "" {
		body["clientOrderId"] = strings.TrimSpace(req.ClientOrderID)
	}
	if req.TimeInForce != "" {
		body["timeInForce"] = strings.ToUpper(strings.TrimSpace(req.TimeInForce))
	}
	if req.ConfirmHighValueOrder {
		body["confirmHighValueOrder"] = true
	}
	if amount := firstNonEmpty(req.OrderAmountDecimal, decimalString(req.OrderAmount)); amount != "" {
		body["orderAmount"] = amount
	} else {
		qty := firstNonEmpty(req.QuantityDecimal, strconv.FormatInt(req.Quantity, 10))
		if req.Quantity <= 0 && strings.TrimSpace(req.QuantityDecimal) == "" {
			return nil, fmt.Errorf("%w: quantity or order_amount is required", broker.ErrInvalidOrderRequest)
		}
		body["quantity"] = qty
	}
	if req.Type == broker.OrderTypeLimit {
		price := decimalString(req.Price)
		if price == "" {
			return nil, fmt.Errorf("%w: limit price is required", broker.ErrInvalidOrderRequest)
		}
		body["price"] = price
	}
	return body, nil
}

func buildModifyOrderBody(req broker.ModifyOrderRequest) (map[string]interface{}, error) {
	orderType := req.Type
	if orderType == "" {
		if req.Price > 0 {
			orderType = broker.OrderTypeLimit
		} else {
			orderType = broker.OrderTypeMarket
		}
	}
	tossType, err := tossOrderType(orderType)
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"orderType": tossType,
	}
	if qty := firstNonEmpty(req.QuantityDecimal, positiveIntString(req.Quantity)); qty != "" {
		body["quantity"] = qty
	}
	if req.Price > 0 {
		body["price"] = decimalString(req.Price)
	}
	if req.ConfirmHighValueOrder {
		body["confirmHighValueOrder"] = true
	}
	return body, nil
}

func tossOrderSide(side broker.OrderSide) (string, error) {
	switch side {
	case broker.OrderSideBuy:
		return "BUY", nil
	case broker.OrderSideSell:
		return "SELL", nil
	default:
		return "", fmt.Errorf("%w: unsupported Toss order side %q", broker.ErrInvalidOrderRequest, side)
	}
}

func tossOrderType(orderType broker.OrderType) (string, error) {
	switch orderType {
	case broker.OrderTypeLimit:
		return "LIMIT", nil
	case broker.OrderTypeMarket:
		return "MARKET", nil
	default:
		return "", fmt.Errorf("%w: unsupported Toss order type %q", broker.ErrInvalidOrderRequest, orderType)
	}
}

func orderToResult(order toss.Order) *broker.OrderResult {
	quantity := decimalToInt64(order.Quantity)
	filledQty := decimalToInt64(order.Execution.FilledQuantity)
	remaining := quantity - filledQty
	if remaining < 0 {
		remaining = 0
	}
	return &broker.OrderResult{
		OrderID:        order.OrderID,
		Status:         mapOrderStatus(order.Status),
		FilledQuantity: filledQty,
		RemainingQty:   remaining,
		AvgFilledPrice: parseOptionalDecimal(order.Execution.AverageFilledPrice),
		Timestamp:      parseTime(firstNonEmpty(order.OrderedAt, stringPtrValue(order.CanceledAt))),
	}
}

func mapOrderStatus(status string) broker.OrderStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FILLED":
		return broker.OrderStatusFilled
	case "CANCELED", "CANCELLED":
		return broker.OrderStatusCancelled
	case "REJECTED", "CANCEL_REJECTED", "REPLACE_REJECTED":
		return broker.OrderStatusRejected
	default:
		return broker.OrderStatusPending
	}
}

func tossInterval(interval string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(interval)) {
	case "", "1d", "d", "day", "daily":
		return "1d", nil
	case "1m", "m", "min", "minute":
		return "1m", nil
	default:
		return "", fmt.Errorf("%w: Toss supports only 1d and 1m candles", broker.ErrUpstreamBadRequest)
	}
}

func requestQuantity(req broker.OrderRequest) int64 {
	if strings.TrimSpace(req.QuantityDecimal) != "" {
		return decimalToInt64(req.QuantityDecimal)
	}
	return req.Quantity
}

func requestModifyQuantity(req broker.ModifyOrderRequest) int64 {
	if strings.TrimSpace(req.QuantityDecimal) != "" {
		return decimalToInt64(req.QuantityDecimal)
	}
	return req.Quantity
}

func parseDecimal(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func parseOptionalDecimal(s *string) float64 {
	if s == nil {
		return 0
	}
	return parseDecimal(*s)
}

func parseDecimalInt64(s string) int64 {
	return decimalToInt64(s)
}

func decimalToInt64(s string) int64 {
	f := parseDecimal(s)
	if f <= 0 {
		return 0
	}
	return int64(math.Floor(f))
}

func decimalString(v float64) string {
	if v <= 0 {
		return ""
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func positiveIntString(v int64) string {
	if v <= 0 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}

func parseOptionalTime(s *string) time.Time {
	if s == nil {
		return time.Time{}
	}
	return parseTime(*s)
}

func parseOptionalStringTime(s *string) time.Time {
	if s == nil {
		return time.Time{}
	}
	return parseTime(*s)
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), t.Location())
}

func marketFromCountry(country string) string {
	switch strings.ToUpper(strings.TrimSpace(country)) {
	case "KR":
		return "KRX"
	case "US":
		return "US"
	default:
		return strings.ToUpper(strings.TrimSpace(country))
	}
}

func marketFromCurrency(currency string) string {
	if strings.EqualFold(strings.TrimSpace(currency), "USD") {
		return "US"
	}
	return "KRX"
}

func countryFromMarket(market string) string {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "KOSPI", "KOSDAQ", "KR_ETC":
		return "KR"
	case "NYSE", "NASDAQ", "AMEX", "US_ETC":
		return "US"
	default:
		return ""
	}
}

func assetTypeFromHolding(item toss.HoldingsItem) broker.AssetType {
	if strings.EqualFold(item.MarketCountry, "US") {
		return broker.AssetOverseas
	}
	return broker.AssetStock
}

func assetTypeFromStock(item toss.StockInfo) broker.AssetType {
	typ := strings.ToUpper(strings.TrimSpace(item.SecurityType))
	switch {
	case strings.Contains(typ, "ETF"), strings.Contains(typ, "FUND"), strings.Contains(typ, "ETN"), strings.Contains(typ, "REIT"):
		return broker.AssetFund
	case countryFromMarket(item.Market) == "US":
		return broker.AssetOverseas
	default:
		return broker.AssetStock
	}
}

func stockSuspended(item toss.StockInfo) bool {
	if item.KoreanMarketDetail == nil {
		return false
	}
	return item.KoreanMarketDetail.KRXTradingSuspended ||
		(item.KoreanMarketDetail.NXTTradingSuspended != nil && *item.KoreanMarketDetail.NXTTradingSuspended)
}

func multiCurrencyMap(amount toss.MultiCurrencyAmount) map[string]float64 {
	out := map[string]float64{"KRW": parseDecimal(amount.KRW)}
	if amount.USD != nil {
		out["USD"] = parseDecimal(*amount.USD)
	}
	return out
}

func cloneCurrencyMap(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
