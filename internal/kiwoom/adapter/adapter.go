package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"

	"github.com/smallfish06/krsec/internal/kiwoom"
	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

// Adapter adapts Kiwoom APIs into broker.Broker.
type Adapter struct {
	client     *kiwoom.Client
	accountID  string
	sandbox    bool
	orderDir   string
	dispatcher *endpointDispatcher
	logger     *slog.Logger

	mu     sync.RWMutex
	orders map[string]orderContext
}

type orderContext struct {
	OrderID      string             `json:"order_id"`
	Symbol       string             `json:"symbol"`
	Exchange     string             `json:"exchange"`
	Side         broker.OrderSide   `json:"side"`
	Quantity     int64              `json:"quantity"`
	RemainingQty int64              `json:"remaining_qty"`
	Price        float64            `json:"price"`
	Status       broker.OrderStatus `json:"status"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

// NewAdapterWithOptions creates a Kiwoom adapter with injectable internals.
func NewAdapterWithOptions(
	sandbox bool,
	accountID string,
	tokenManager tokencache.Manager,
	orderContextDir string,
	logger *slog.Logger,
) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	client := kiwoom.NewClientWithTokenManager(sandbox, tokenManager)
	client.SetLogger(logger)
	a := &Adapter{
		client:    client,
		accountID: strings.TrimSpace(accountID),
		sandbox:   sandbox,
		orderDir:  strings.TrimSpace(orderContextDir),
		orders:    make(map[string]orderContext),
		logger:    logger,
	}
	a.dispatcher = newEndpointDispatcher(a)
	if err := a.loadOrderContexts(); err != nil {
		logger.Warn("failed to load persisted orders", "account", a.accountID, "error", err)
	}
	return a
}

// Name returns broker name.
func (a *Adapter) Name() string {
	return broker.NameKiwoom
}

// Authenticate authenticates with Kiwoom.
func (a *Adapter) Authenticate(ctx context.Context, creds broker.Credentials) (*broker.Token, error) {
	return a.client.Authenticate(ctx, creds)
}

// GetQuote retrieves quote for domestic markets.
func (a *Adapter) GetQuote(ctx context.Context, market, symbol string) (*broker.Quote, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	if _, err := toKiwoomExchange(market); err != nil {
		return nil, err
	}

	quote, err := a.client.InquirePrice(ctx, symbol)
	if err != nil {
		return nil, err
	}

	price := normalizedPrice(parseFloatString(quote.CurPrc))
	prevClose := normalizedPrice(parseFloatString(quote.BasePric))
	change := parseFloatString(quote.PredPre)
	if prevClose == 0 && (price != 0 || change != 0) {
		prevClose = price - change
	}

	symbolOut := normalizeSymbol(quote.StkCd)
	if symbolOut == "" {
		symbolOut = symbol
	}

	return &broker.Quote{
		Symbol:     symbolOut,
		Market:     normalizeOutputMarket(market),
		Price:      price,
		Open:       normalizedPrice(parseFloatString(quote.OpenPric)),
		High:       normalizedPrice(parseFloatString(quote.HighPric)),
		Low:        normalizedPrice(parseFloatString(quote.LowPric)),
		Close:      price,
		PrevClose:  prevClose,
		Change:     change,
		ChangeRate: parseFloatString(quote.FluRt),
		Volume:     parseIntString(quote.TrdeQty),
		UpperLimit: normalizedPrice(parseFloatString(quote.UplPric)),
		LowerLimit: normalizedPrice(parseFloatString(quote.LstPric)),
		Timestamp:  time.Now(),
	}, nil
}

// GetOHLCV retrieves domestic OHLCV for day/week/month intervals.
func (a *Adapter) GetOHLCV(ctx context.Context, market, symbol string, opts broker.OHLCVOpts) ([]broker.OHLCV, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}
	if _, err := toKiwoomExchange(market); err != nil {
		return nil, err
	}

	interval := strings.ToLower(strings.TrimSpace(opts.Interval))
	if interval == "" {
		interval = "1d"
	}

	baseDate := time.Now().Format("20060102")
	if !opts.To.IsZero() {
		baseDate = opts.To.Format("20060102")
	}

	var (
		rows []map[string]any
		err  error
	)

	switch interval {
	case "1d", "d", "day", "daily":
		resp, e := a.client.InquireDailyPrice(ctx, symbol, baseDate)
		err = e
		if e == nil {
			rows = decodeObjectArray(resp.StkDtPoleChartQry)
		}
	case "1w", "w", "week", "weekly":
		resp, e := a.client.InquireWeeklyPrice(ctx, symbol, baseDate)
		err = e
		if e == nil {
			rows = decodeObjectArray(resp.StkStkPoleChartQry)
		}
	case "1mo", "mo", "month", "monthly":
		resp, e := a.client.InquireMonthlyPrice(ctx, symbol, baseDate)
		err = e
		if e == nil {
			rows = decodeObjectArray(resp.StkMthPoleChartQry)
		}
	default:
		return nil, fmt.Errorf("unsupported interval for kiwoom: %s", opts.Interval)
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []broker.OHLCV{}, nil
	}

	out := make([]broker.OHLCV, 0, len(rows))
	for _, row := range rows {
		dt, ok := parseDateYYYYMMDDString(asAnyString(row["dt"]))
		if !ok {
			continue
		}
		item := broker.OHLCV{
			Timestamp: dt,
			Open:      normalizedPrice(asAnyFloat(row["open_pric"])),
			High:      normalizedPrice(asAnyFloat(row["high_pric"])),
			Low:       normalizedPrice(asAnyFloat(row["low_pric"])),
			Close:     normalizedPrice(asAnyFloat(row["cur_prc"])),
			Volume:    asAnyInt(row["trde_qty"]),
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

// GetBalance retrieves account balance summary.
func (a *Adapter) GetBalance(ctx context.Context, accountID string) (*broker.Balance, error) {
	if strings.TrimSpace(accountID) == "" {
		accountID = a.accountID
	}

	bal, err := a.client.InquireBalance(ctx, "KRX")
	if err != nil {
		return nil, err
	}

	deposit := parseFloatString(bal.Entr)
	evaluationTotal := parseFloatString(bal.EvltAmtTot)
	totalAssets := deposit + evaluationTotal

	return &broker.Balance{
		AccountID:        strings.TrimSpace(accountID),
		Cash:             deposit,
		TotalAssets:      totalAssets,
		BuyingPower:      parseFloatString(bal.OrdAlowa),
		WithdrawableCash: parseFloatString(bal.OrdAlowa),
		ReceivableAmount: parseFloatString(bal.EntrD2),
		ProfitLoss:       parseFloatString(bal.TotPlTot),
		ProfitLossPct:    parseFloatString(bal.TotPlRt),
		PositionCost:     parseFloatString(bal.StkBuyTotAmt),
		PositionValue:    evaluationTotal,
		SettlementT1:     parseFloatString(bal.EntrD1),
		Unsettled:        parseFloatString(bal.UnclStkAmt),
		LoanBalance:      parseFloatString(bal.CrdLoanTot),
	}, nil
}

// GetPositions retrieves account stock positions.
func (a *Adapter) GetPositions(ctx context.Context, _ string) ([]broker.Position, error) {
	positionsResp, err := a.client.InquirePositions(ctx, "0", "KRX")
	if err != nil {
		return nil, err
	}
	rows := decodeObjectArray(positionsResp.AcntEvltRemnIndvTot)
	if len(rows) == 0 {
		return []broker.Position{}, nil
	}

	positions := make([]broker.Position, 0, len(rows))
	for _, row := range rows {
		symbol := normalizeSymbol(asAnyString(row["stk_cd"]))
		if symbol == "" {
			continue
		}
		remainingQty := asAnyInt(row["rmnd_qty"])
		if remainingQty == 0 {
			continue
		}

		positions = append(positions, broker.Position{
			Symbol:        symbol,
			Name:          asAnyString(row["stk_nm"]),
			Market:        "KRX",
			MarketCode:    "KRX",
			AssetType:     broker.AssetStock,
			Quantity:      remainingQty,
			OrderableQty:  asAnyInt(row["trde_able_qty"]),
			TodayBuyQty:   asAnyInt(row["tdy_buyq"]),
			TodaySellQty:  asAnyInt(row["tdy_sellq"]),
			AvgPrice:      normalizedPrice(asAnyFloat(row["pur_pric"])),
			CurrentPrice:  normalizedPrice(asAnyFloat(row["cur_prc"])),
			PurchaseValue: normalizedPrice(asAnyFloat(row["pur_amt"])),
			MarketValue:   normalizedPrice(asAnyFloat(row["evlt_amt"])),
			ProfitLoss:    asAnyFloat(row["evltv_prft"]),
			ProfitLossPct: asAnyFloat(row["prft_rt"]),
			WeightPct:     asAnyFloat(row["poss_rt"]),
			LoanDate:      asAnyString(row["crd_loan_dt"]),
		})
	}
	return positions, nil
}

// PlaceOrder places buy/sell stock orders.
func (a *Adapter) PlaceOrder(ctx context.Context, req broker.OrderRequest) (*broker.OrderResult, error) {
	symbol := normalizeSymbol(req.Symbol)
	if symbol == "" || req.Quantity <= 0 {
		return nil, broker.ErrInvalidOrderRequest
	}
	exchange, err := toKiwoomExchange(req.Market)
	if err != nil {
		return nil, err
	}

	tradeType, orderPrice, err := toTradeTypeAndPrice(req.Type, req.Price)
	if err != nil {
		return nil, err
	}

	side := kiwoom.StockOrderSideBuy
	if req.Side == broker.OrderSideSell {
		side = kiwoom.StockOrderSideSell
	}

	ack, err := a.client.PlaceStockOrder(ctx, side, kiwoomspecs.KiwoomApiDostkOrdrKt10000Request{
		DmstStexTp: exchange,
		StkCd:      symbol,
		OrdQty:     fmt.Sprintf("%d", req.Quantity),
		OrdUv:      orderPrice,
		TrdeTp:     tradeType,
		CondUv:     "",
	})
	if err != nil {
		return nil, err
	}

	orderID := strings.TrimSpace(ack.OrdNo)
	if orderID == "" {
		return nil, fmt.Errorf("missing order id in kiwoom response")
	}

	a.storeOrderContext(orderID, orderContext{
		OrderID:      orderID,
		Symbol:       symbol,
		Exchange:     exchange,
		Side:         req.Side,
		Quantity:     req.Quantity,
		RemainingQty: req.Quantity,
		Price:        req.Price,
		Status:       broker.OrderStatusPending,
		UpdatedAt:    time.Now(),
	})

	return &broker.OrderResult{
		OrderID:      orderID,
		Status:       broker.OrderStatusPending,
		RemainingQty: req.Quantity,
		Message:      "",
		Timestamp:    time.Now(),
	}, nil
}

// CancelOrder cancels a pending order.
func (a *Adapter) CancelOrder(ctx context.Context, orderID string) error {
	meta, err := a.resolveOrderContext(ctx, orderID)
	if err != nil {
		return err
	}
	cancelQty := meta.RemainingQty
	if cancelQty <= 0 {
		cancelQty = meta.Quantity
	}
	if cancelQty <= 0 {
		cancelQty = 1
	}

	ack, err := a.client.CancelStockOrder(ctx, kiwoomspecs.KiwoomApiDostkOrdrKt10003Request{
		DmstStexTp: meta.Exchange,
		OrigOrdNo:  meta.OrderID,
		StkCd:      meta.Symbol,
		CnclQty:    fmt.Sprintf("%d", cancelQty),
	})
	if err != nil {
		return err
	}

	meta.Status = broker.OrderStatusCancelled
	meta.RemainingQty = 0
	meta.UpdatedAt = time.Now()
	a.storeOrderContext(meta.OrderID, meta)

	newID := strings.TrimSpace(ack.OrdNo)
	if newID != "" && newID != meta.OrderID {
		meta.OrderID = newID
		a.storeOrderContext(newID, meta)
	}
	return nil
}

// ModifyOrder modifies an existing order.
func (a *Adapter) ModifyOrder(ctx context.Context, orderID string, req broker.ModifyOrderRequest) (*broker.OrderResult, error) {
	meta, err := a.resolveOrderContext(ctx, orderID)
	if err != nil {
		return nil, err
	}

	newQty := meta.Quantity
	if req.Quantity > 0 {
		newQty = req.Quantity
	}
	newPrice := meta.Price
	if req.Price > 0 {
		newPrice = req.Price
	}
	if newQty <= 0 || newPrice <= 0 {
		return nil, broker.ErrInvalidOrderRequest
	}

	ack, err := a.client.ModifyStockOrder(ctx, kiwoomspecs.KiwoomApiDostkOrdrKt10002Request{
		DmstStexTp: meta.Exchange,
		OrigOrdNo:  meta.OrderID,
		StkCd:      meta.Symbol,
		MdfyQty:    fmt.Sprintf("%d", newQty),
		MdfyUv:     formatPrice(newPrice),
		MdfyCondUv: "",
	})
	if err != nil {
		return nil, err
	}

	newOrderID := strings.TrimSpace(ack.OrdNo)
	if newOrderID == "" {
		newOrderID = orderID
	}

	meta.OrderID = newOrderID
	meta.Quantity = newQty
	meta.RemainingQty = newQty
	meta.Price = newPrice
	meta.Status = broker.OrderStatusPending
	meta.UpdatedAt = time.Now()
	a.storeOrderContext(newOrderID, meta)
	if newOrderID != orderID {
		a.storeOrderContext(orderID, meta)
	}

	return &broker.OrderResult{
		OrderID:      newOrderID,
		Status:       broker.OrderStatusPending,
		RemainingQty: newQty,
		Message:      "",
		Timestamp:    time.Now(),
	}, nil
}

// GetOrder returns order status by order id.
func (a *Adapter) GetOrder(ctx context.Context, orderID string) (*broker.OrderResult, error) {
	if strings.TrimSpace(orderID) == "" {
		return nil, broker.ErrInvalidOrderRequest
	}

	meta, _ := a.getOrderContext(orderID)
	unsettled, err := a.fetchUnsettled(ctx, meta.Symbol)
	if err == nil {
		for _, row := range unsettled {
			if strings.TrimSpace(asAnyString(row["ord_no"])) != strings.TrimSpace(orderID) {
				continue
			}
			ordQty := asAnyInt(row["ord_qty"])
			remaining := asAnyInt(row["oso_qty"])
			filled := max(ordQty-remaining, 0)
			status := mapOrderStatus(asAnyString(row["ord_stt"]), remaining)
			result := &broker.OrderResult{
				OrderID:        orderID,
				Status:         status,
				FilledQuantity: filled,
				RemainingQty:   remaining,
				AvgFilledPrice: normalizedPrice(asAnyFloat(row["cntr_pric"])),
				Message:        asAnyString(row["ord_stt"]),
				Timestamp:      time.Now(),
			}
			meta.OrderID = orderID
			meta.Symbol = normalizeSymbol(asAnyString(row["stk_cd"]))
			meta.Exchange = exchangeFromUnsettledRow(row)
			meta.Quantity = ordQty
			meta.RemainingQty = remaining
			meta.Price = normalizedPrice(asAnyFloat(row["ord_pric"]))
			meta.Side = sideFromKiwoomOrderText(asAnyString(row["io_tp_nm"]))
			meta.Status = status
			meta.UpdatedAt = time.Now()
			a.storeOrderContext(orderID, meta)
			return result, nil
		}
	}

	fills, fillErr := a.GetOrderFills(ctx, orderID)
	if fillErr == nil && len(fills) > 0 {
		var qty int64
		var amount float64
		for _, f := range fills {
			qty += f.Quantity
			amount += float64(f.Quantity) * f.Price
		}
		avg := 0.0
		if qty > 0 {
			avg = amount / float64(qty)
		}
		return &broker.OrderResult{
			OrderID:        orderID,
			Status:         broker.OrderStatusFilled,
			FilledQuantity: qty,
			RemainingQty:   0,
			AvgFilledPrice: avg,
			Timestamp:      time.Now(),
		}, nil
	}

	if meta.OrderID != "" {
		return &broker.OrderResult{
			OrderID:      orderID,
			Status:       meta.Status,
			RemainingQty: meta.RemainingQty,
			Timestamp:    time.Now(),
		}, nil
	}
	return nil, broker.ErrOrderNotFound
}

// GetOrderFills returns fills for the order.
func (a *Adapter) GetOrderFills(ctx context.Context, orderID string) ([]broker.OrderFill, error) {
	if strings.TrimSpace(orderID) == "" {
		return nil, broker.ErrInvalidOrderRequest
	}
	meta, hasContext := a.getOrderContext(orderID)

	executionResp, err := a.client.InquireOrderExecutions(ctx, meta.Symbol)
	if err != nil {
		return nil, err
	}
	executions := decodeObjectArray(executionResp.Cntr)
	if len(executions) == 0 {
		if hasContext {
			return []broker.OrderFill{}, nil
		}
		return nil, broker.ErrOrderNotFound
	}

	fills := make([]broker.OrderFill, 0)
	for _, row := range executions {
		if strings.TrimSpace(asAnyString(row["ord_no"])) != strings.TrimSpace(orderID) {
			continue
		}
		qty := asAnyInt(row["cntr_qty"])
		if qty <= 0 {
			continue
		}
		price := normalizedPrice(asAnyFloat(row["cntr_pric"]))
		fills = append(fills, broker.OrderFill{
			OrderID:   orderID,
			Symbol:    normalizeSymbol(asAnyString(row["stk_cd"])),
			Market:    mapStexCodeToMarket(asAnyString(row["stex_tp"]), asAnyString(row["stex_tp_txt"])),
			Side:      sideFromKiwoomText(asAnyString(row["io_tp_nm"])),
			Quantity:  qty,
			Price:     price,
			Amount:    float64(qty) * price,
			Currency:  "KRW",
			FilledAt:  parseOrderTime(asAnyString(row["ord_tm"])),
			RawStatus: strings.TrimSpace(asAnyString(row["ord_stt"])),
		})
	}

	if len(fills) == 0 {
		if hasContext {
			return []broker.OrderFill{}, nil
		}
		return nil, broker.ErrOrderNotFound
	}
	sort.Slice(fills, func(i, j int) bool {
		return fills[i].FilledAt.Before(fills[j].FilledAt)
	})
	return fills, nil
}

// GetInstrument retrieves normalized instrument metadata.
func (a *Adapter) GetInstrument(ctx context.Context, market, symbol string) (*broker.Instrument, error) {
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return nil, broker.ErrInvalidSymbol
	}

	info, err := a.client.InquireInstrumentInfo(ctx, symbol)
	if err != nil {
		return nil, err
	}

	marketOut := normalizeOutputMarket(market)
	if marketOut == "" {
		marketOut = marketFromCode(info.Marketcode, info.Marketname)
	}

	state := strings.TrimSpace(info.State)
	listed := !strings.Contains(state, "상장폐지")
	suspended := strings.Contains(state, "거래정지") || strings.Contains(state, "정리매매")

	return &broker.Instrument{
		Symbol:       normalizeSymbol(info.Code),
		Market:       marketOut,
		Name:         firstNonEmpty(info.Name, symbol),
		Exchange:     firstNonEmpty(info.Marketname, marketOut),
		Currency:     "KRW",
		Country:      "KR",
		AssetType:    broker.AssetStock,
		Sector:       info.Upname,
		ListedShares: parseIntString(info.Listcount),
		IsListed:     listed,
		IsSuspended:  suspended,
		ListingDate:  info.Regday,
	}, nil
}

func (a *Adapter) fetchUnsettled(ctx context.Context, symbol string) ([]map[string]any, error) {
	resp, err := a.client.InquireUnsettledOrders(ctx, symbol)
	if err != nil {
		return nil, err
	}
	return decodeObjectArray(resp.Oso), nil
}

func (a *Adapter) storeOrderContext(orderID string, meta orderContext) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return
	}
	meta.OrderID = orderID
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = time.Now()
	}
	a.mu.Lock()
	a.orders[orderID] = meta
	a.compactOrderContextsLocked(maxPersistedOrderContexts)
	a.mu.Unlock()
	_ = a.persistOrderContexts()
}

func (a *Adapter) getOrderContext(orderID string) (orderContext, bool) {
	a.mu.RLock()
	meta, ok := a.orders[strings.TrimSpace(orderID)]
	a.mu.RUnlock()
	return meta, ok
}

func (a *Adapter) resolveOrderContext(ctx context.Context, orderID string) (orderContext, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return orderContext{}, broker.ErrInvalidOrderRequest
	}
	if meta, ok := a.getOrderContext(orderID); ok {
		return meta, nil
	}

	rows, err := a.fetchUnsettled(ctx, "")
	if err != nil {
		return orderContext{}, err
	}
	for _, row := range rows {
		if strings.TrimSpace(asAnyString(row["ord_no"])) != orderID {
			continue
		}
		remainingQty := asAnyInt(row["oso_qty"])
		meta := orderContext{
			OrderID:      orderID,
			Symbol:       normalizeSymbol(asAnyString(row["stk_cd"])),
			Exchange:     exchangeFromUnsettledRow(row),
			Quantity:     asAnyInt(row["ord_qty"]),
			RemainingQty: remainingQty,
			Price:        normalizedPrice(asAnyFloat(row["ord_pric"])),
			Side:         sideFromKiwoomOrderText(asAnyString(row["io_tp_nm"])),
			Status:       mapOrderStatus(asAnyString(row["ord_stt"]), remainingQty),
			UpdatedAt:    time.Now(),
		}
		a.storeOrderContext(orderID, meta)
		return meta, nil
	}
	return orderContext{}, broker.ErrOrderNotFound
}

func toKiwoomExchange(market string) (string, error) {
	m := strings.ToUpper(strings.TrimSpace(market))
	if m == "" {
		return "KRX", nil
	}
	switch m {
	case "KR", "KRX", "KOSPI", "KOSDAQ":
		return "KRX", nil
	case "NXT":
		return "NXT", nil
	case "SOR":
		return "SOR", nil
	default:
		return "", broker.ErrInvalidMarket
	}
}

func toTradeTypeAndPrice(orderType broker.OrderType, price float64) (string, string, error) {
	switch orderType {
	case broker.OrderTypeMarket:
		return "3", "", nil
	case broker.OrderTypeLimit:
		if price <= 0 {
			return "", "", broker.ErrInvalidOrderRequest
		}
		return "0", formatPrice(price), nil
	default:
		return "", "", broker.ErrInvalidOrderRequest
	}
}

func formatPrice(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func normalizeSymbol(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	s = strings.TrimPrefix(s, "A")
	return s
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 23, 59, 59, int(time.Second-time.Nanosecond), t.Location())
}

func mapOrderStatus(raw string, remaining int64) broker.OrderStatus {
	r := strings.TrimSpace(raw)
	switch {
	case strings.Contains(r, "취소"):
		return broker.OrderStatusCancelled
	case strings.Contains(r, "거부") || strings.Contains(r, "실패"):
		return broker.OrderStatusRejected
	case strings.Contains(r, "체결") && remaining == 0:
		return broker.OrderStatusFilled
	default:
		return broker.OrderStatusPending
	}
}

func exchangeFromUnsettledRow(row map[string]any) string {
	if txt := strings.ToUpper(strings.TrimSpace(asAnyString(row["stex_tp_txt"]))); txt != "" {
		switch txt {
		case "KRX", "NXT", "SOR":
			return txt
		}
	}
	return mapStexCodeToMarket(asAnyString(row["stex_tp"]), asAnyString(row["stex_tp_txt"]))
}

func mapStexCodeToMarket(code, text string) string {
	switch strings.TrimSpace(code) {
	case "1":
		return "KRX"
	case "2":
		return "NXT"
	case "0":
		if strings.EqualFold(strings.TrimSpace(text), "SOR") {
			return "SOR"
		}
		return "KRX"
	}
	t := strings.ToUpper(strings.TrimSpace(text))
	if t == "" {
		return "KRX"
	}
	return t
}

func sideFromKiwoomText(v string) string {
	raw := strings.TrimSpace(v)
	switch {
	case strings.Contains(raw, "매수"):
		return string(broker.OrderSideBuy)
	case strings.Contains(raw, "매도"):
		return string(broker.OrderSideSell)
	default:
		return ""
	}
}

func sideFromKiwoomOrderText(v string) broker.OrderSide {
	if strings.Contains(strings.TrimSpace(v), "매도") {
		return broker.OrderSideSell
	}
	return broker.OrderSideBuy
}

func parseFloatString(raw string) float64 {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseIntString(raw string) int64 {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return 0
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f)
}

func asAnyString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func asAnyFloat(v any) float64 {
	return parseFloatString(asAnyString(v))
}

func asAnyInt(v any) int64 {
	return parseIntString(asAnyString(v))
}

func parseDateYYYYMMDDString(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("20060102", value)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func decodeObjectArray(raw any) []map[string]any {
	switch t := raw.(type) {
	case nil:
		return nil
	case json.RawMessage:
		return decodeObjectArrayFromJSON(t)
	case []byte:
		return decodeObjectArrayFromJSON(t)
	case []map[string]any:
		return t
	}

	rv := reflect.ValueOf(raw)
	if !rv.IsValid() {
		return nil
	}
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil
	}

	out := make([]map[string]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()
		switch row := item.(type) {
		case map[string]any:
			out = append(out, row)
		default:
			encoded, err := json.Marshal(item)
			if err != nil || len(encoded) == 0 {
				continue
			}
			m := make(map[string]any)
			if err := json.Unmarshal(encoded, &m); err != nil {
				continue
			}
			out = append(out, m)
		}
	}
	return out
}

func decodeObjectArrayFromJSON(raw []byte) []map[string]any {
	if len(raw) == 0 {
		return nil
	}
	items := make([]map[string]any, 0)
	if err := json.Unmarshal(raw, &items); err == nil {
		return items
	}
	generic := make([]any, 0)
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(generic))
	for _, item := range generic {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, row)
	}
	return out
}

func parseOrderTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if len(v) == 6 {
		now := time.Now()
		parsed, err := time.ParseInLocation("150405", v, now.Location())
		if err == nil {
			return time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), 0, now.Location())
		}
	}
	return time.Now()
}

func normalizeOutputMarket(market string) string {
	m := strings.ToUpper(strings.TrimSpace(market))
	switch m {
	case "KR", "KOSPI", "KOSDAQ", "":
		return "KRX"
	case "NXT", "SOR", "KRX":
		return m
	default:
		return m
	}
}

func marketFromCode(code, name string) string {
	switch strings.TrimSpace(code) {
	case "0", "10":
		return "KRX"
	case "30":
		return "KONEX"
	}
	if strings.Contains(strings.TrimSpace(name), "코스닥") {
		return "KRX"
	}
	return "KRX"
}

func firstNonEmpty(values ...string) string {
	first, ok := lo.Find(values, func(v string) bool {
		return strings.TrimSpace(v) != ""
	})
	if ok {
		return strings.TrimSpace(first)
	}
	return ""
}

func normalizedPrice(v float64) float64 {
	return math.Abs(v)
}
