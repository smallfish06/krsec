// Example: embed the krsec server with a custom in-memory broker.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
	apiserver "github.com/smallfish06/krsec/pkg/server"
)

type demoOrder struct {
	req        broker.OrderRequest
	result     broker.OrderResult
	modifiedAt time.Time
}

type demoBroker struct {
	mu     sync.RWMutex
	orders map[string]demoOrder
	seq    int64
}

func newDemoBroker() *demoBroker {
	return &demoBroker{
		orders: make(map[string]demoOrder),
	}
}

func (b *demoBroker) Name() string {
	return "DEMO"
}

func (b *demoBroker) Authenticate(_ context.Context, creds broker.Credentials) (*broker.Token, error) {
	if strings.TrimSpace(creds.AppKey) == "" || strings.TrimSpace(creds.AppSecret) == "" {
		return nil, broker.ErrInvalidCredentials
	}

	return &broker.Token{
		AccessToken: "demo-access-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}, nil
}

func (b *demoBroker) GetQuote(_ context.Context, market, symbol string) (*broker.Quote, error) {
	if strings.TrimSpace(market) == "" {
		return nil, broker.ErrInvalidMarket
	}
	if strings.TrimSpace(symbol) == "" {
		return nil, broker.ErrInvalidSymbol
	}

	price := 70000.0
	now := time.Now().UTC()
	return &broker.Quote{
		Symbol:    symbol,
		Market:    strings.ToUpper(market),
		Price:     price,
		Open:      price - 500,
		High:      price + 1200,
		Low:       price - 900,
		Close:     price - 200,
		Volume:    1234567,
		Timestamp: now,
	}, nil
}

func (b *demoBroker) GetOHLCV(_ context.Context, market, symbol string, opts broker.OHLCVOpts) ([]broker.OHLCV, error) {
	if strings.TrimSpace(market) == "" {
		return nil, broker.ErrInvalidMarket
	}
	if strings.TrimSpace(symbol) == "" {
		return nil, broker.ErrInvalidSymbol
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	base := 70000.0
	out := make([]broker.OHLCV, 0, limit)
	now := time.Now().UTC()
	for i := limit - 1; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * 24 * time.Hour)
		openPx := base + float64((i%7)-3)*120
		closePx := openPx + float64((i%5)-2)*80
		high := max(openPx, closePx) + 90
		low := min(openPx, closePx) - 110
		out = append(out, broker.OHLCV{
			Timestamp: t,
			Open:      openPx,
			High:      high,
			Low:       low,
			Close:     closePx,
			Volume:    100000 + int64(i*777),
		})
	}
	return out, nil
}

func (b *demoBroker) GetBalance(_ context.Context, accountID string) (*broker.Balance, error) {
	if strings.TrimSpace(accountID) == "" {
		return nil, broker.ErrInvalidOrderRequest
	}
	return &broker.Balance{
		AccountID:     accountID,
		Cash:          3000000,
		TotalAssets:   9500000,
		BuyingPower:   6000000,
		ProfitLoss:    350000,
		ProfitLossPct: 3.82,
		PositionCost:  9150000,
		PositionValue: 9500000,
	}, nil
}

func (b *demoBroker) GetPositions(_ context.Context, _ string) ([]broker.Position, error) {
	return []broker.Position{
		{
			Symbol:        "005930",
			Name:          "삼성전자",
			Market:        "KRX",
			AssetType:     broker.AssetStock,
			Quantity:      12,
			OrderableQty:  12,
			AvgPrice:      68000,
			CurrentPrice:  70000,
			PurchaseValue: 816000,
			MarketValue:   840000,
			ProfitLoss:    24000,
			ProfitLossPct: 2.94,
		},
	}, nil
}

func (b *demoBroker) PlaceOrder(_ context.Context, req broker.OrderRequest) (*broker.OrderResult, error) {
	if strings.TrimSpace(req.Symbol) == "" || strings.TrimSpace(req.Market) == "" || req.Quantity <= 0 {
		return nil, broker.ErrInvalidOrderRequest
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.seq++
	orderID := "DEMO-" + strconv.FormatInt(b.seq, 10)
	status := broker.OrderStatusPending
	if req.Type == broker.OrderTypeMarket {
		status = broker.OrderStatusFilled
	}

	result := broker.OrderResult{
		OrderID:   orderID,
		Status:    status,
		Message:   "accepted",
		Timestamp: time.Now().UTC(),
	}
	b.orders[orderID] = demoOrder{
		req:        req,
		result:     result,
		modifiedAt: time.Now().UTC(),
	}
	return &result, nil
}

func (b *demoBroker) CancelOrder(_ context.Context, orderID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	ord, ok := b.orders[orderID]
	if !ok {
		return broker.ErrOrderNotFound
	}
	ord.result.Status = broker.OrderStatusCancelled
	ord.result.Message = "cancelled"
	ord.modifiedAt = time.Now().UTC()
	b.orders[orderID] = ord
	return nil
}

func (b *demoBroker) ModifyOrder(_ context.Context, orderID string, req broker.ModifyOrderRequest) (*broker.OrderResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ord, ok := b.orders[orderID]
	if !ok {
		return nil, broker.ErrOrderNotFound
	}
	if req.Quantity > 0 {
		ord.req.Quantity = req.Quantity
	}
	if req.Price > 0 {
		ord.req.Price = req.Price
	}
	ord.result.Message = "modified"
	ord.modifiedAt = time.Now().UTC()
	b.orders[orderID] = ord
	return &ord.result, nil
}

// Optional capability for GET /accounts/{account_id}/orders/{order_id}
func (b *demoBroker) GetOrder(_ context.Context, orderID string) (*broker.OrderResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	ord, ok := b.orders[orderID]
	if !ok {
		return nil, broker.ErrOrderNotFound
	}
	result := ord.result
	return &result, nil
}

// Optional capability for GET /accounts/{account_id}/orders/{order_id}/fills
func (b *demoBroker) GetOrderFills(_ context.Context, orderID string) ([]broker.OrderFill, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	ord, ok := b.orders[orderID]
	if !ok {
		return nil, broker.ErrOrderNotFound
	}
	if ord.result.Status != broker.OrderStatusFilled {
		return []broker.OrderFill{}, nil
	}

	price := ord.req.Price
	if price <= 0 {
		price = 70000
	}
	fill := broker.OrderFill{
		OrderID:  ord.result.OrderID,
		Symbol:   ord.req.Symbol,
		Market:   ord.req.Market,
		Side:     string(ord.req.Side),
		Quantity: ord.req.Quantity,
		Price:    price,
		Amount:   float64(ord.req.Quantity) * price,
		Currency: "KRW",
		FilledAt: ord.modifiedAt,
	}
	return []broker.OrderFill{fill}, nil
}

// Optional capability for GET /instruments/{market}/{symbol}
func (b *demoBroker) GetInstrument(_ context.Context, market, symbol string) (*broker.Instrument, error) {
	if strings.TrimSpace(market) == "" {
		return nil, broker.ErrInvalidMarket
	}
	if strings.TrimSpace(symbol) == "" {
		return nil, broker.ErrInvalidSymbol
	}
	return &broker.Instrument{
		Symbol:          symbol,
		Market:          strings.ToUpper(market),
		Name:            "Demo Instrument",
		NameEn:          "Demo Instrument",
		Exchange:        "KRX",
		Currency:        "KRW",
		Country:         "KR",
		AssetType:       broker.AssetStock,
		ProductType:     "stock",
		ProductTypeCode: "300",
		IsListed:        true,
	}, nil
}

func main() {
	host := flag.String("host", "0.0.0.0", "HTTP bind host")
	port := flag.Int("port", 18090, "HTTP bind port")
	accountID := flag.String("account-id", "demo-acc-1", "Demo account ID")
	flag.Parse()

	demo := newDemoBroker()

	srv := apiserver.New(apiserver.Options{
		Host: *host,
		Port: *port,
		Accounts: []apiserver.Account{
			{
				ID:     *accountID,
				Name:   "Demo Account",
				Broker: "demo",
				Credentials: broker.Credentials{
					AppKey:    "demo-key",
					AppSecret: "demo-secret",
				},
			},
		},
		Brokers: map[string]broker.Broker{
			*accountID: demo,
		},
	})

	slog.Info("custom broker server listening", "url", "http://"+*host+":"+strconv.Itoa(*port))
	slog.Info("OpenAPI JSON", "url", "http://"+*host+":"+strconv.Itoa(*port)+"/swagger/openapi.json")
	slog.Info("Swagger UI", "url", "http://"+*host+":"+strconv.Itoa(*port)+"/swagger/")
	slog.Info("try", "cmd", "curl http://"+*host+":"+strconv.Itoa(*port)+"/quotes/KRX/005930")
	slog.Info("try", "cmd", "curl http://"+*host+":"+strconv.Itoa(*port)+"/accounts/"+*accountID+"/balance")
	slog.Info("try", "cmd", "curl -X POST http://"+*host+":"+strconv.Itoa(*port)+"/accounts/"+*accountID+"/orders -H 'Content-Type: application/json' -d '"+sampleOrder()+"'")

	if err := srv.Run(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func sampleOrder() string {
	return `{"symbol":"005930","market":"KRX","side":"buy","type":"market","quantity":1}`
}

