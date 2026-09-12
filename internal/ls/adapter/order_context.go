package adapter

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/smallfish06/krsec/internal/orderctxstore"
	"github.com/smallfish06/krsec/pkg/broker"
)

var lsOrderLocation = time.FixedZone("KST", 9*60*60)

type orderContext struct {
	OrderID      string           `json:"order_id"`
	AccountID    string           `json:"account_id"`
	CredentialID string           `json:"credential_id"`
	Symbol       string           `json:"symbol"`
	Market       string           `json:"market"`
	Side         broker.OrderSide `json:"side"`
	OrderType    broker.OrderType `json:"order_type"`
	Quantity     int64            `json:"quantity"`
	Price        float64          `json:"price"`
	PlacedAt     time.Time        `json:"placed_at"`
}

func credentialFingerprint(key string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(key))))
}

func (a *Adapter) orderNow() time.Time {
	if a.now != nil {
		return a.now().In(lsOrderLocation)
	}
	return time.Now().In(lsOrderLocation)
}

func (a *Adapter) orderContextPath() (string, error) {
	dir := a.orderDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".krsec", "orders")
	}
	env := "real"
	if a.sandbox {
		env = "sandbox"
	}
	return filepath.Join(dir, fmt.Sprintf("ls-%s-%x.json", env, sha256.Sum256([]byte(a.accountID)))), nil
}

func (a *Adapter) loadOrderContexts() error {
	path, err := a.orderContextPath()
	if err != nil {
		return err
	}
	return orderctxstore.Load(path, a.orders, 300, func(meta orderContext) time.Time { return meta.PlacedAt })
}

func (a *Adapter) orderContextReady() error {
	if a.orderLoadErr != nil {
		return fmt.Errorf("%w: LS order context could not be loaded", broker.ErrServerError)
	}
	if a.accountID == "" || a.credentialID == "" {
		return fmt.Errorf("%w: LS order account identity is unavailable", broker.ErrUnauthorized)
	}
	return nil
}

func (a *Adapter) storeOrderContext(id string, meta orderContext) {
	id = normalizeOrderID(id)
	if id == "" {
		return
	}
	meta.OrderID = id
	a.orderMu.Lock()
	defer a.orderMu.Unlock()
	if a.orders == nil {
		a.orders = make(map[string]orderContext)
	}
	a.orders[id] = meta
	orderctxstore.Compact(a.orders, 300, func(meta orderContext) time.Time { return meta.PlacedAt })
	path, err := a.orderContextPath()
	if err == nil {
		err = orderctxstore.Persist(path, a.orders)
	}
	if err != nil {
		// The upstream order has already been accepted. Do not report failure
		// that could make a caller place a duplicate order.
		a.logger.Warn("LS order accepted; context persistence failed", "error", err)
	}
}

func (a *Adapter) trackedOrder(id string) (orderContext, error) {
	if err := a.orderContextReady(); err != nil {
		return orderContext{}, err
	}
	id = normalizeOrderID(id)
	if id == "" {
		return orderContext{}, broker.ErrOrderNotFound
	}
	a.orderMu.Lock()
	meta, ok := a.orders[id]
	a.orderMu.Unlock()
	if !ok || meta.AccountID != a.accountID || meta.CredentialID != a.credentialID || meta.Symbol == "" {
		return orderContext{}, fmt.Errorf("%w: LS requires an order placed through this account's common API", broker.ErrNotSupported)
	}
	if meta.PlacedAt.In(lsOrderLocation).Format(time.DateOnly) != a.orderNow().Format(time.DateOnly) {
		return orderContext{}, fmt.Errorf("%w: LS common order management is limited to the recorded trading date", broker.ErrNotSupported)
	}
	return meta, nil
}

func normalizeOrderID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) == 0 || len(id) > 10 {
		return ""
	}
	for _, ch := range id {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil || n == 0 {
		return ""
	}
	return strconv.FormatUint(n, 10)
}

func canonicalOrderSymbol(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if len(symbol) == 7 && symbol[0] == 'A' {
		symbol = symbol[1:]
	}
	if len(symbol) != 6 {
		return ""
	}
	for _, ch := range symbol {
		if (ch < '0' || ch > '9') && (ch < 'A' || ch > 'Z') {
			return ""
		}
	}
	return symbol
}

func lsWireOrderSymbol(symbol string, sandbox bool) string {
	if sandbox {
		return "A" + symbol
	}
	return symbol
}

func lsOrderMarket(market string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "", "KRX", "KOSPI", "KOSDAQ", "K":
		return "KRX", nil
	case "NXT", "N":
		return "NXT", nil
	default:
		return "", fmt.Errorf("%w: LS common orders require an explicit KRX or NXT market", broker.ErrInvalidMarket)
	}
}
