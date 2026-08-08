package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/smallfish06/krsec/internal/kis"
	"github.com/smallfish06/krsec/internal/kiwoom"
	internalls "github.com/smallfish06/krsec/internal/ls"
	pkgadapter "github.com/smallfish06/krsec/pkg/adapter"
	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
	pkgkis "github.com/smallfish06/krsec/pkg/kis"
	pkgkiwoom "github.com/smallfish06/krsec/pkg/kiwoom"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
	pkgls "github.com/smallfish06/krsec/pkg/ls"
	tokencache "github.com/smallfish06/krsec/pkg/token"
	pkgtoss "github.com/smallfish06/krsec/pkg/toss"
)

type smokeResult struct {
	Broker   string
	Account  string
	CaseName string
	Status   string
	Duration time.Duration
	Detail   string
}

type endpointCase struct {
	Name        string
	Method      string
	Path        string
	TRID        string // KIS/LS: tr_id/tr_cd, Kiwoom: api_id
	Fields      map[string]string
	ExpectError bool
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	var results []smokeResult

	kisTokenManager := pkgkis.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	kiwoomTokenManager := pkgkiwoom.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	lsTokenManager := pkgls.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	tossTokenManager := pkgtoss.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)

	for _, acc := range cfg.Accounts {
		switch acc.Broker {
		case broker.CodeKIS:
			runKISAccount(&results, acc, kisTokenManager, cfg.Storage.OrderContextDir)
		case broker.CodeKiwoom:
			runKiwoomAccount(&results, acc, kiwoomTokenManager, cfg.Storage.OrderContextDir)
		case broker.CodeLS:
			runLSAccount(&results, acc, lsTokenManager)
		case broker.CodeToss:
			runTossAccount(&results, acc, tossTokenManager)
		default:
			results = append(results, smokeResult{
				Broker:   strings.ToUpper(acc.Broker),
				Account:  acc.AccountID,
				CaseName: "bootstrap",
				Status:   "SKIP",
				Detail:   "unsupported broker in test runner",
			})
		}
	}

	printSummary(results)

	failed := 0
	for _, r := range results {
		if r.Status == "FAIL" || r.Status == "UNEXPECTED_SUCCESS" {
			failed++
		}
	}
	if failed > 0 {
		os.Exit(2)
	}
}

func runKISAccount(results *[]smokeResult, acc config.AccountConfig, tm tokencache.Manager, orderContextDir string) {
	a := pkgkis.NewAdapterWithOptions(acc.Sandbox, acc.AccountID, pkgadapter.Options{
		TokenManager:    tm,
		OrderContextDir: orderContextDir,
	})
	creds := broker.Credentials{AppKey: acc.AppKey, AppSecret: acc.AppSecret}

	runCase(results, "KIS", acc.AccountID, "auth", false, func(ctx context.Context) error {
		_, err := a.Authenticate(ctx, creds)
		return err
	})

	runCase(results, "KIS", acc.AccountID, "GetQuote(KRX,005930)", false, func(ctx context.Context) error {
		_, err := a.GetQuote(ctx, "KRX", "005930")
		return err
	})
	runCase(results, "KIS", acc.AccountID, "GetOHLCV(KRX,005930,1d)", false, func(ctx context.Context) error {
		_, err := a.GetOHLCV(ctx, "KRX", "005930", broker.OHLCVOpts{Interval: "1d", Limit: 10})
		return err
	})
	runCase(results, "KIS", acc.AccountID, "GetBalance", false, func(ctx context.Context) error {
		_, err := a.GetBalance(ctx, acc.AccountID)
		return err
	})
	runCase(results, "KIS", acc.AccountID, "GetPositions", false, func(ctx context.Context) error {
		_, err := a.GetPositions(ctx, acc.AccountID)
		return err
	})
	runCase(results, "KIS", acc.AccountID, "GetInstrument(KRX,005930)", false, func(ctx context.Context) error {
		_, err := a.GetInstrument(ctx, "KRX", "005930")
		return err
	})
	runCase(results, "KIS", acc.AccountID, "GetInstrument(US,AAPL)", false, func(ctx context.Context) error {
		_, err := a.GetInstrument(ctx, "US", "AAPL")
		return err
	})

	runCase(results, "KIS", acc.AccountID, "GetOrder(fake)", true, func(ctx context.Context) error {
		_, err := a.GetOrder(ctx, "0000000000")
		return err
	})
	runCase(results, "KIS", acc.AccountID, "GetOrderFills(fake)", true, func(ctx context.Context) error {
		_, err := a.GetOrderFills(ctx, "0000000000")
		return err
	})

	fromDate := time.Now().AddDate(0, -1, 0).Format("20060102")
	toDate := time.Now().AddDate(0, 0, -1).Format("20060102")
	cano, acntPrdtCd := splitKISAccountID(acc.AccountID)

	for _, tc := range buildKISEndpointCases(fromDate, toDate, cano, acntPrdtCd) {
		runCase(results, "KIS", acc.AccountID, "CallEndpoint "+tc.Name, tc.ExpectError, func(ctx context.Context) error {
			_, err := a.CallEndpoint(ctx, tc.Method, tc.Path, tc.TRID, cloneMap(tc.Fields))
			return err
		})
	}
}

func runKiwoomAccount(results *[]smokeResult, acc config.AccountConfig, tm tokencache.Manager, orderContextDir string) {
	a := pkgkiwoom.NewAdapterWithOptions(acc.Sandbox, acc.AccountID, pkgadapter.Options{
		TokenManager:    tm,
		OrderContextDir: orderContextDir,
	})
	creds := broker.Credentials{AppKey: acc.AppKey, AppSecret: acc.AppSecret}

	nextAllowed := time.Now()
	runKiwoomCase := func(caseName string, expectError bool, fn func(context.Context) error) {
		if wait := time.Until(nextAllowed); wait > 0 {
			time.Sleep(wait)
		}
		runCase(results, "KIWOOM", acc.AccountID, caseName, expectError, fn)
		nextAllowed = time.Now().Add(120 * time.Millisecond)
	}

	runKiwoomCase("auth", false, func(ctx context.Context) error {
		_, err := a.Authenticate(ctx, creds)
		return err
	})

	runKiwoomCase("GetQuote(KRX,005930)", false, func(ctx context.Context) error {
		_, err := a.GetQuote(ctx, "KRX", "005930")
		return err
	})
	runKiwoomCase("GetOHLCV(KRX,005930,1d)", false, func(ctx context.Context) error {
		_, err := a.GetOHLCV(ctx, "KRX", "005930", broker.OHLCVOpts{Interval: "1d", Limit: 10})
		return err
	})
	runKiwoomCase("GetOHLCV(KRX,005930,1w)", false, func(ctx context.Context) error {
		_, err := a.GetOHLCV(ctx, "KRX", "005930", broker.OHLCVOpts{Interval: "1w", Limit: 10})
		return err
	})
	runKiwoomCase("GetOHLCV(KRX,005930,1mo)", false, func(ctx context.Context) error {
		_, err := a.GetOHLCV(ctx, "KRX", "005930", broker.OHLCVOpts{Interval: "1mo", Limit: 10})
		return err
	})
	if acc.Sandbox {
		*results = append(*results, smokeResult{
			Broker:   "KIWOOM",
			Account:  acc.AccountID,
			CaseName: "GetBalance",
			Status:   "SKIP",
			Detail:   "not provided in kiwoom sandbox (RC9000)",
		})
	} else {
		runKiwoomCase("GetBalance", false, func(ctx context.Context) error {
			_, err := a.GetBalance(ctx, acc.AccountID)
			return err
		})
	}
	runKiwoomCase("GetPositions", false, func(ctx context.Context) error {
		_, err := a.GetPositions(ctx, acc.AccountID)
		return err
	})
	runKiwoomCase("GetInstrument(KRX,005930)", false, func(ctx context.Context) error {
		_, err := a.GetInstrument(ctx, "KRX", "005930")
		return err
	})

	// CallEndpoint tests for all Kiwoom API_IDs
	for _, tc := range buildKiwoomEndpointCases(acc.Sandbox) {
		runKiwoomCase("CallEndpoint "+tc.Name, tc.ExpectError, func(ctx context.Context) error {
			_, err := a.CallEndpoint(ctx, tc.Method, tc.Path, tc.TRID, cloneMap(tc.Fields))
			return err
		})
	}

	// Fake order/cancel tests
	runKiwoomCase("GetOrder(fake)", true, func(ctx context.Context) error {
		_, err := a.GetOrder(ctx, "0000000000")
		return err
	})
	runKiwoomCase("GetOrderFills(fake)", true, func(ctx context.Context) error {
		_, err := a.GetOrderFills(ctx, "0000000000")
		return err
	})
	runKiwoomCase("CancelOrder(fake)", true, func(ctx context.Context) error {
		return a.CancelOrder(ctx, "0000000000")
	})
	runKiwoomCase("ModifyOrder(fake)", true, func(ctx context.Context) error {
		_, err := a.ModifyOrder(ctx, "0000000000", broker.ModifyOrderRequest{Price: 1000, Quantity: 1})
		return err
	})
	runKiwoomCase("PlaceOrder(dry-run invalid)", true, func(ctx context.Context) error {
		_, err := a.PlaceOrder(ctx, broker.OrderRequest{
			AccountID: acc.AccountID,
			Market:    "KRX",
			Symbol:    "000000",
			Side:      broker.OrderSideBuy,
			Type:      broker.OrderTypeLimit,
			Quantity:  1,
			Price:     1,
		})
		return err
	})
}

func runLSAccount(results *[]smokeResult, acc config.AccountConfig, tm tokencache.Manager) {
	a := pkgls.NewAdapterWithOptions(acc.Sandbox, acc.AccountID, pkgls.Options{
		Options:    pkgadapter.Options{TokenManager: tm},
		MACAddress: acc.MACAddress,
	})
	creds := broker.Credentials{AppKey: acc.AppKey, AppSecret: acc.AppSecret}

	runCase(results, "LS", acc.AccountID, "auth", false, func(ctx context.Context) error {
		_, err := a.Authenticate(ctx, creds)
		return err
	})
	runCase(results, "LS", acc.AccountID, "GetQuote(KRX,078020)", false, func(ctx context.Context) error {
		_, err := a.GetQuote(ctx, "KRX", "078020")
		return err
	})
	runCase(results, "LS", acc.AccountID, "GetQuote(US-NASDAQ,AAPL)", false, func(ctx context.Context) error {
		_, err := a.GetQuote(ctx, "US-NASDAQ", "AAPL")
		return err
	})
	runCase(results, "LS", acc.AccountID, "GetOHLCV(KRX,078020,1d)", false, func(ctx context.Context) error {
		_, err := a.GetOHLCV(ctx, "KRX", "078020", broker.OHLCVOpts{Interval: "1d", Limit: 10})
		return err
	})
	runCase(results, "LS", acc.AccountID, "GetOHLCV(US-NASDAQ,AAPL,1d)", false, func(ctx context.Context) error {
		_, err := a.GetOHLCV(ctx, "US-NASDAQ", "AAPL", broker.OHLCVOpts{Interval: "1d", Limit: 5})
		return err
	})
	runCase(results, "LS", acc.AccountID, "GetInstrument(KRX,078020)", false, func(ctx context.Context) error {
		_, err := a.GetInstrument(ctx, "KRX", "078020")
		return err
	})
	runCase(results, "LS", acc.AccountID, "GetInstrument(US-NASDAQ,AAPL)", false, func(ctx context.Context) error {
		_, err := a.GetInstrument(ctx, "US-NASDAQ", "AAPL")
		return err
	})
	runCase(results, "LS", acc.AccountID, "CallEndpoint t1102", false, func(ctx context.Context) error {
		_, err := a.CallEndpoint(ctx, http.MethodPost, internalls.PathStockMarket, internalls.TRStockQuote, map[string]any{
			"t1102InBlock": map[string]any{
				"shcode":    "078020",
				"exchgubun": "K",
			},
		})
		return err
	})
	runCase(results, "LS", acc.AccountID, "CallEndpoint g3101", false, func(ctx context.Context) error {
		_, err := a.CallEndpoint(ctx, http.MethodPost, internalls.PathOverseasStockMarket, internalls.TROverseasStockQuote, map[string]any{
			"g3101InBlock": map[string]any{
				"delaygb":   "R",
				"keysymbol": "82AAPL",
				"exchcd":    "82",
				"symbol":    "AAPL",
			},
		})
		return err
	})
	runCase(results, "LS", acc.AccountID, "BuildTradeSubscriptions", false, func(ctx context.Context) error {
		subs, err := a.BuildTradeSubscriptions(ctx)
		if err != nil {
			return err
		}
		if len(subs) == 0 {
			return fmt.Errorf("no realtime subscriptions built")
		}
		return nil
	})
	runCase(results, "LS", acc.AccountID, "BuildOverseasTradeSubscriptions(US-NASDAQ)", false, func(ctx context.Context) error {
		subs, err := a.BuildOverseasTradeSubscriptions(ctx, "US-NASDAQ", 5)
		if err != nil {
			return err
		}
		if len(subs) == 0 {
			return fmt.Errorf("no overseas realtime subscriptions built")
		}
		return nil
	})
	runCase(results, "LS", acc.AccountID, "RealtimeSubscribe(S3_,078020)", false, func(ctx context.Context) error {
		conn, err := a.ConnectRealtime(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()

		if err := conn.Subscribe(ctx, internalls.TRRealtimeKOSPI, "078020"); err != nil {
			return err
		}

		readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		_, err = conn.Read(readCtx)
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		return err
	})
	runCase(results, "LS", acc.AccountID, "RealtimeSubscribe(GSC,82AAPL)", false, func(ctx context.Context) error {
		conn, err := a.ConnectRealtime(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()

		key, err := pkgls.OverseasRealtimeKey("82", "AAPL")
		if err != nil {
			return err
		}
		if err := conn.Subscribe(ctx, internalls.TRRealtimeOverseasTrade, key); err != nil {
			return err
		}

		readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		_, err = conn.Read(readCtx)
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		return err
	})
}

func runTossAccount(results *[]smokeResult, acc config.AccountConfig, tm tokencache.Manager) {
	a := pkgtoss.NewAdapterWithOptions(acc.Sandbox, acc.AccountID, acc.AccountSeq, pkgadapter.Options{
		TokenManager: tm,
	})
	creds := broker.Credentials{AppKey: acc.AppKey, AppSecret: acc.AppSecret}

	if !runCase(results, "TOSS", acc.AccountID, "auth", false, func(ctx context.Context) error {
		_, err := a.Authenticate(ctx, creds)
		return err
	}) {
		return
	}
	runCase(results, "TOSS", acc.AccountID, "CallEndpoint accounts", false, func(ctx context.Context) error {
		_, err := a.CallEndpoint(ctx, http.MethodGet, "/api/v1/accounts", nil, nil)
		return err
	})
	runCase(results, "TOSS", acc.AccountID, "CallEndpoint prices(005930,AAPL)", false, func(ctx context.Context) error {
		_, err := a.CallEndpoint(ctx, http.MethodGet, "/api/v1/prices", map[string]any{
			"symbols": "005930,AAPL",
		}, nil)
		return err
	})
	runCase(results, "TOSS", acc.AccountID, "CallEndpoint stocks(005930,AAPL)", false, func(ctx context.Context) error {
		_, err := a.CallEndpoint(ctx, http.MethodGet, "/api/v1/stocks", map[string]any{
			"symbols": "005930,AAPL",
		}, nil)
		return err
	})
	runCase(results, "TOSS", acc.AccountID, "CallEndpoint holdings", false, func(ctx context.Context) error {
		_, err := a.CallEndpoint(ctx, http.MethodGet, "/api/v1/holdings", nil, nil)
		return err
	})
	runCase(results, "TOSS", acc.AccountID, "GetQuote(KRX,005930)", false, func(ctx context.Context) error {
		_, err := a.GetQuote(ctx, "KRX", "005930")
		return err
	})
	runCase(results, "TOSS", acc.AccountID, "GetOHLCV(KRX,005930,1d)", false, func(ctx context.Context) error {
		_, err := a.GetOHLCV(ctx, "KRX", "005930", broker.OHLCVOpts{Interval: "1d", Limit: 10})
		return err
	})
	runCase(results, "TOSS", acc.AccountID, "GetInstrument(KRX,005930)", false, func(ctx context.Context) error {
		_, err := a.GetInstrument(ctx, "KRX", "005930")
		return err
	})
	runCase(results, "TOSS", acc.AccountID, "GetBalance", false, func(ctx context.Context) error {
		_, err := a.GetBalance(ctx, acc.AccountID)
		return err
	})
	runCase(results, "TOSS", acc.AccountID, "GetPositions", false, func(ctx context.Context) error {
		_, err := a.GetPositions(ctx, acc.AccountID)
		return err
	})
}

func runCase(results *[]smokeResult, brokerName, accountID, caseName string, expectError bool, fn func(context.Context) error) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	start := time.Now()
	err := fn(ctx)
	dur := time.Since(start)

	status := "PASS"
	detail := ""
	if expectError {
		if err == nil {
			status = "UNEXPECTED_SUCCESS"
			detail = "expected an error for safe dry-run"
		} else {
			status = "EXPECTED_ERROR"
			detail = shorten(err.Error(), 180)
		}
	} else {
		if err != nil {
			if brokerName == "KIS" && isKnownKISAccountUnavailable(err) {
				status = "EXPECTED_ERROR"
				detail = shorten(err.Error(), 220)
			} else {
				status = "FAIL"
				detail = shorten(err.Error(), 220)
			}
		}
	}

	result := smokeResult{
		Broker:   brokerName,
		Account:  accountID,
		CaseName: caseName,
		Status:   status,
		Duration: dur,
		Detail:   detail,
	}
	*results = append(*results, result)
	printCaseProgress(result)
	return status != "FAIL" && status != "UNEXPECTED_SUCCESS"
}

func printCaseProgress(result smokeResult) {
	fmt.Printf("[%s] %s %s (%s)\n", result.Status, result.Broker, result.CaseName, result.Duration.Round(time.Millisecond))
}

func buildKISEndpointCases(fromDate, toDate, cano, acntPrdtCd string) []endpointCase {
	return []endpointCase{
		{Name: "inquire-price", Method: http.MethodGet, Path: kis.PathDomesticStockInquirePrice, TRID: "FHKST01010100", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "J", "FID_INPUT_ISCD": "005930"}},
		{Name: "inquire-daily-price", Method: http.MethodGet, Path: kis.PathDomesticStockInquireDailyPrice, TRID: "FHKST01010400", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "J", "FID_INPUT_ISCD": "005930", "FID_PERIOD_DIV_CODE": "D", "FID_ORG_ADJ_PRC": "0"}},
		{Name: "inquire-asking-price", Method: http.MethodGet, Path: kis.PathDomesticStockInquireAskingPriceExpCcn, TRID: "FHKST01010200", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "J", "FID_INPUT_ISCD": "005930"}},
		{Name: "inquire-ccnl", Method: http.MethodGet, Path: kis.PathDomesticStockInquireCcnl, TRID: "FHKST01010300", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "J", "FID_INPUT_ISCD": "005930"}},
		{Name: "inquire-time-conclusion", Method: http.MethodGet, Path: kis.PathDomesticStockInquireTimeItemConclusion, TRID: "FHPST01060000", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "J", "FID_INPUT_ISCD": "005930", "FID_INPUT_HOUR_1": "153000"}},
		{Name: "inquire-member", Method: http.MethodGet, Path: kis.PathDomesticStockInquireMember, TRID: "FHKST01010600", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "J", "FID_INPUT_ISCD": "005930"}},
		{Name: "etfetn-component", Method: http.MethodGet, Path: kis.PathETFETNComponentStockPrice, TRID: "FHKST121600C0", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "J", "FID_INPUT_ISCD": "069500", "FID_COND_SCR_DIV_CODE": "11216"}},
		{Name: "volume-rank", Method: http.MethodGet, Path: kis.PathDomesticStockVolumeRank, TRID: "FHPST01710000", Fields: map[string]string{"FID_BLNG_CLS_CODE": "0", "FID_COND_MRKT_DIV_CODE": "J", "FID_COND_SCR_DIV_CODE": "20171", "FID_DIV_CLS_CODE": "0", "FID_INPUT_DATE_1": "", "FID_INPUT_ISCD": "0000", "FID_INPUT_PRICE_1": "", "FID_INPUT_PRICE_2": "", "FID_TRGT_CLS_CODE": "0", "FID_TRGT_EXLS_CLS_CODE": "0", "FID_VOL_CNT": "0"}},
		{Name: "market-cap", Method: http.MethodGet, Path: kis.PathDomesticStockRankingMarketCap, TRID: "FHPST01740000", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "J", "FID_COND_SCR_DIV_CODE": "20174", "FID_DIV_CLS_CODE": "0", "FID_INPUT_ISCD": "0000", "FID_INPUT_PRICE_1": "", "FID_INPUT_PRICE_2": "", "FID_TRGT_CLS_CODE": "0", "FID_TRGT_EXLS_CLS_CODE": "0", "FID_VOL_CNT": "0"}},
		{Name: "fluctuation", Method: http.MethodGet, Path: kis.PathDomesticStockRankingFluctuation, TRID: "FHPST01700000", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "J", "FID_COND_SCR_DIV_CODE": "20170", "FID_DIV_CLS_CODE": "0", "FID_INPUT_CNT_1": "0", "FID_INPUT_ISCD": "0000", "FID_INPUT_PRICE_1": "", "FID_INPUT_PRICE_2": "", "FID_PRC_CLS_CODE": "0", "FID_RANK_SORT_CLS_CODE": "0", "FID_RSFL_RATE1": "", "FID_RSFL_RATE2": "", "FID_TRGT_CLS_CODE": "0", "FID_TRGT_EXLS_CLS_CODE": "0", "FID_VOL_CNT": "0"}},
		{Name: "index-price", Method: http.MethodGet, Path: kis.PathDomesticStockInquireIndexPrice, TRID: "FHPUP02100000", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "U", "FID_INPUT_ISCD": "0001"}},
		{Name: "index-daily", Method: http.MethodGet, Path: kis.PathDomesticStockInquireIndexDailyPrice, TRID: "FHPUP02120000", Fields: map[string]string{"FID_PERIOD_DIV_CODE": "D", "FID_COND_MRKT_DIV_CODE": "U", "FID_INPUT_ISCD": "0001", "FID_INPUT_DATE_1": fromDate}},
		{Name: "daily-indexchart", Method: http.MethodGet, Path: kis.PathDomesticStockInquireDailyIndexChart, TRID: "FHKUP03500100", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "U", "FID_INPUT_ISCD": "0001", "FID_INPUT_DATE_1": fromDate, "FID_INPUT_DATE_2": toDate, "FID_PERIOD_DIV_CODE": "D"}},
		{Name: "ovrs-price", Method: http.MethodGet, Path: kis.PathOverseasPricePrice, TRID: "HHDFS00000300", Fields: map[string]string{"AUTH": "", "EXCD": "NAS", "SYMB": "AAPL"}},
		{Name: "ovrs-daily-chart", Method: http.MethodGet, Path: kis.PathOverseasPriceInquireDailyChartPrice, TRID: "FHKST03030100", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "N", "FID_INPUT_ISCD": "AAPL", "FID_INPUT_DATE_1": fromDate, "FID_INPUT_DATE_2": toDate, "FID_PERIOD_DIV_CODE": "D"}},
		{Name: "ovrs-daily-price", Method: http.MethodGet, Path: kis.PathOverseasPriceDailyPrice, TRID: "HHDFS76240000", Fields: map[string]string{"AUTH": "", "EXCD": "NAS", "SYMB": "AAPL", "GUBN": "0", "BYMD": toDate, "MODP": "0"}},
		{Name: "ovrs-price-detail", Method: http.MethodGet, Path: kis.PathOverseasPricePriceDetail, TRID: "HHDFS76200200", Fields: map[string]string{"AUTH": "", "EXCD": "NAS", "SYMB": "AAPL"}},
		{Name: "ovrs-ccnl", Method: http.MethodGet, Path: kis.PathOverseasPriceInquireCcnl, TRID: "HHDFS76200300", Fields: map[string]string{"EXCD": "NAS", "TDAY": "0", "SYMB": "AAPL", "AUTH": "", "KEYB": ""}},
		{Name: "ovrs-updown-rate", Method: http.MethodGet, Path: kis.PathOverseasStockRankingUpdownRate, TRID: "HHDFS76290000", Fields: map[string]string{"EXCD": "NAS", "NDAY": "0", "GUBN": "1", "VOL_RANG": "0", "AUTH": "", "KEYB": ""}},
		{Name: "ovrs-time-itemchart", Method: http.MethodGet, Path: kis.PathOverseasPriceInquireTimeItemChart, TRID: "HHDFS76950200", Fields: map[string]string{"AUTH": "", "EXCD": "NAS", "SYMB": "AAPL", "NMIN": "1", "PINC": "1", "NEXT": "0", "NREC": "120", "FILL": "", "KEYB": ""}},
		{Name: "bond-price", Method: http.MethodGet, Path: kis.PathDomesticBondInquirePrice, TRID: "FHKBJ773400C0", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "B", "FID_INPUT_ISCD": "KR103501GE04"}},
		{Name: "bond-daily-price", Method: http.MethodGet, Path: kis.PathDomesticBondInquireDailyPrice, TRID: "FHKBJ773404C0", Fields: map[string]string{"FID_COND_MRKT_DIV_CODE": "B", "FID_INPUT_ISCD": "KR2033022D33", "FID_INPUT_DATE_1": fromDate, "FID_INPUT_DATE_2": toDate, "FID_PERIOD_DIV_CODE": "D", "FID_ORG_ADJ_PRC": "0"}},
		{Name: "bond-search-info", Method: http.MethodGet, Path: kis.PathDomesticBondSearchBondInfo, TRID: "CTPF1114R", Fields: map[string]string{"PDNO": "KR2033022D33", "PRDT_TYPE_CD": "302"}},
		{Name: "bond-avg-unit", Method: http.MethodGet, Path: kis.PathDomesticBondAvgUnit, TRID: "CTPF2005R", Fields: map[string]string{"INQR_STRT_DT": fromDate, "INQR_END_DT": toDate, "PDNO": "KR2033022D33", "PRDT_TYPE_CD": "302", "VRFC_KIND_CD": "00", "CTX_AREA_NK30": "", "CTX_AREA_FK100": ""}},
		{Name: "bond-balance", Method: http.MethodGet, Path: kis.PathDomesticBondInquireBalance, TRID: "CTSC8407R", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "INQR_CNDT": "00", "PDNO": "", "BUY_DT": "", "CTX_AREA_FK200": "", "CTX_AREA_NK200": ""}},
		{Name: "search-stock-info", Method: http.MethodGet, Path: kis.PathDomesticStockSearchStockInfo, TRID: "CTPF1604R", Fields: map[string]string{"PDNO": "005930", "PRDT_TYPE_CD": "300"}},
		{Name: "search-info", Method: http.MethodGet, Path: kis.PathDomesticStockSearchInfo, TRID: "CTPF1002R", Fields: map[string]string{"PDNO": "005930", "PRDT_TYPE_CD": "300"}},
		{Name: "ovrs-search-info", Method: http.MethodGet, Path: kis.PathOverseasPriceSearchInfo, TRID: "CTPF1702R", Fields: map[string]string{"PDNO": "AAPL", "PRDT_TYPE_CD": "512"}},
		{Name: "trade-inquire-balance", Method: http.MethodGet, Path: kis.PathDomesticStockTradingInquireBalance, TRID: "TTTC8434R", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "AFHR_FLPR_YN": "N", "INQR_DVSN": "01", "UNPR_DVSN": "01", "FUND_STTL_ICLD_YN": "N", "FNCG_AMT_AUTO_RDPT_YN": "N", "PRCS_DVSN": "00"}},
		{Name: "ovrs-trade-balance", Method: http.MethodGet, Path: kis.PathOverseasStockTradingInquireBalance, TRID: "TTTS3012R", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "OVRS_EXCG_CD": "NASD", "TR_CRCY_CD": "USD", "CTX_AREA_FK200": "", "CTX_AREA_NK200": ""}},
		{Name: "ovrs-psamount", Method: http.MethodGet, Path: kis.PathOverseasStockTradingInquirePsAmount, TRID: "TTTS3007R", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "OVRS_EXCG_CD": "NASD", "OVRS_ORD_UNPR": "100", "ITEM_CD": "AAPL"}},
		{Name: "psbl-order", Method: http.MethodGet, Path: kis.PathDomesticStockTradingInquirePsblOrder, TRID: "TTTC8908R", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "PDNO": "005930", "ORD_UNPR": "70000", "ORD_DVSN": "00", "CMA_EVLU_AMT_ICLD_YN": "Y", "OVRS_ICLD_YN": "N"}},
		{Name: "period-trade-profit", Method: http.MethodGet, Path: kis.PathDomesticStockTradingInquirePeriodTradeProfit, TRID: "TTTC8715R", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "SORT_DVSN": "00", "INQR_STRT_DT": fromDate, "INQR_END_DT": toDate, "CBLC_DVSN": "00", "PDNO": "", "CTX_AREA_NK100": "", "CTX_AREA_FK100": ""}},
		{Name: "daily-ccld", Method: http.MethodGet, Path: kis.PathDomesticStockTradingInquireDailyCcld, TRID: "TTTC0081R", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "INQR_STRT_DT": fromDate, "INQR_END_DT": toDate, "SLL_BUY_DVSN_CD": "00", "INQR_DVSN": "00", "CCLD_DVSN": "00", "INQR_DVSN_3": "00", "INQR_DVSN_1": "", "ORD_GNO_BRNO": "", "ODNO": "", "EXCG_ID_DVSN_CD": "ALL", "CTX_AREA_FK100": "", "CTX_AREA_NK100": ""}},
		{Name: "ovrs-trade-ccnl", Method: http.MethodGet, Path: kis.PathOverseasStockTradingInquireCcnl, TRID: "TTTS3035R", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "CCLD_NCCS_DVSN": "00", "SORT_SQN": "DS", "ORD_STRT_DT": fromDate, "ORD_END_DT": toDate, "ORD_DT": "", "ODNO": "", "ORD_GNO_BRNO": "", "PDNO": "", "SLL_BUY_DVSN": "00", "OVRS_EXCG_CD": "NASD", "CTX_AREA_FK200": "", "CTX_AREA_NK200": ""}},
		{Name: "order-cash", Method: http.MethodPost, Path: kis.PathDomesticStockTradingOrderCash, TRID: "TTTC0802U", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "PDNO": "000000", "ORD_DVSN": "00", "ORD_QTY": "0", "ORD_UNPR": "1"}, ExpectError: true},
		{Name: "order-rvsecncl", Method: http.MethodPost, Path: kis.PathDomesticStockTradingOrderRvseCncl, TRID: "TTTC0803U", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "KRX_FWDG_ORD_ORGNO": "", "ORGN_ODNO": "0000000000", "ORD_DVSN": "00", "RVSE_CNCL_DVSN_CD": "02", "ORD_QTY": "0", "ORD_UNPR": "0", "QTY_ALL_ORD_YN": "N"}, ExpectError: true},
		{Name: "ovrs-order", Method: http.MethodPost, Path: kis.PathOverseasStockTradingOrder, TRID: "TTTT1002U", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "OVRS_EXCG_CD": "NASD", "PDNO": "AAPL", "ORD_QTY": "0", "OVRS_ORD_UNPR": "1", "ORD_DVSN": "00", "ORD_SVR_DVSN_CD": "0"}, ExpectError: true},
		{Name: "ovrs-order-rvsecncl", Method: http.MethodPost, Path: kis.PathOverseasStockTradingOrderRvseCncl, TRID: "TTTT1004U", Fields: map[string]string{"CANO": cano, "ACNT_PRDT_CD": acntPrdtCd, "OVRS_EXCG_CD": "NASD", "PDNO": "AAPL", "ORGN_ODNO": "0000000000", "RVSE_CNCL_DVSN_CD": "02", "ORD_QTY": "0", "OVRS_ORD_UNPR": "1"}, ExpectError: true},
	}
}

func splitKISAccountID(accountID string) (string, string) {
	id := strings.TrimSpace(accountID)
	if id == "" {
		return "", "01"
	}
	if idx := strings.Index(id, "-"); idx > 0 && idx+1 < len(id) {
		return id[:idx], id[idx+1:]
	}
	return id, "01"
}

func buildKiwoomEndpointCases(sandbox bool) []endpointCase {
	cases := []endpointCase{
		// 시세 조회
		{Name: "domestic-quote", Method: http.MethodPost, Path: kiwoom.PathStockInfo, TRID: kiwoomspecs.KiwoomAPIIDKa10001, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "execution-info", Method: http.MethodPost, Path: kiwoom.PathStockInfo, TRID: kiwoomspecs.KiwoomAPIIDKa10003, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "orderbook", Method: http.MethodPost, Path: kiwoom.PathMarketCond, TRID: kiwoomspecs.KiwoomAPIIDKa10004, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "orderbook-level2", Method: http.MethodPost, Path: kiwoom.PathMarketCond, TRID: kiwoomspecs.KiwoomAPIIDKa10005, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "orderbook-level3", Method: http.MethodPost, Path: kiwoom.PathMarketCond, TRID: kiwoomspecs.KiwoomAPIIDKa10006, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "orderbook-level4", Method: http.MethodPost, Path: kiwoom.PathMarketCond, TRID: kiwoomspecs.KiwoomAPIIDKa10007, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "instrument-info", Method: http.MethodPost, Path: kiwoom.PathStockInfo, TRID: kiwoomspecs.KiwoomAPIIDKa10100, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "investor-by-stock", Method: http.MethodPost, Path: kiwoom.PathStockInfo, TRID: kiwoomspecs.KiwoomAPIIDKa10059, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "stock-velocity", Method: http.MethodPost, Path: kiwoom.PathStockInfo, TRID: kiwoomspecs.KiwoomAPIIDKa10095, Fields: map[string]string{"stk_cd": "005930"}},
		// 업종
		{Name: "sector-by-stock", Method: http.MethodPost, Path: kiwoom.PathSector, TRID: kiwoomspecs.KiwoomAPIIDKa10010, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "sector-current", Method: http.MethodPost, Path: kiwoom.PathSector, TRID: kiwoomspecs.KiwoomAPIIDKa20001, Fields: map[string]string{"inds_cd": "001"}},
		{Name: "sector-by-price", Method: http.MethodPost, Path: kiwoom.PathSector, TRID: kiwoomspecs.KiwoomAPIIDKa20002, Fields: map[string]string{"inds_cd": "001"}},
		{Name: "sector-index-price", Method: http.MethodPost, Path: kiwoom.PathSector, TRID: kiwoomspecs.KiwoomAPIIDKa20003, Fields: map[string]string{"inds_cd": "001"}},
		// 랭킹
		{Name: "volume-rank", Method: http.MethodPost, Path: kiwoom.PathRankingInfo, TRID: kiwoomspecs.KiwoomAPIIDKa10030, Fields: map[string]string{}},
		{Name: "change-rate-rank", Method: http.MethodPost, Path: kiwoom.PathRankingInfo, TRID: kiwoomspecs.KiwoomAPIIDKa10027, Fields: map[string]string{}},
		{Name: "investor-rank-by-stock", Method: http.MethodPost, Path: kiwoom.PathRankingInfo, TRID: kiwoomspecs.KiwoomAPIIDKa10040, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "trade-activity-by-stock", Method: http.MethodPost, Path: kiwoom.PathRankingInfo, TRID: kiwoomspecs.KiwoomAPIIDKa10053, Fields: map[string]string{"stk_cd": "005930"}},
		// ELW
		{Name: "elw-detail", Method: http.MethodPost, Path: kiwoom.PathELW, TRID: kiwoomspecs.KiwoomAPIIDKa30012, Fields: map[string]string{"stk_cd": "58F137"}},
		// 계좌
		{Name: "account-balance", Method: http.MethodPost, Path: kiwoom.PathAccount, TRID: kiwoomspecs.KiwoomAPIIDKt00005, Fields: map[string]string{}},
		{Name: "account-positions", Method: http.MethodPost, Path: kiwoom.PathAccount, TRID: kiwoomspecs.KiwoomAPIIDKt00018, Fields: map[string]string{}},
		{Name: "account-deposit", Method: http.MethodPost, Path: kiwoom.PathAccount, TRID: kiwoomspecs.KiwoomAPIIDKt00001, Fields: map[string]string{"qry_tp": "3"}},
		{Name: "account-order-exec-detail", Method: http.MethodPost, Path: kiwoom.PathAccount, TRID: kiwoomspecs.KiwoomAPIIDKt00007, Fields: map[string]string{}},
		{Name: "account-order-exec-status", Method: http.MethodPost, Path: kiwoom.PathAccount, TRID: kiwoomspecs.KiwoomAPIIDKt00009, Fields: map[string]string{}},
		{Name: "account-orderable", Method: http.MethodPost, Path: kiwoom.PathAccount, TRID: kiwoomspecs.KiwoomAPIIDKt00010, Fields: map[string]string{"stk_cd": "005930", "trde_tp": "2", "uv": "1000"}},
		{Name: "account-margin", Method: http.MethodPost, Path: kiwoom.PathAccount, TRID: kiwoomspecs.KiwoomAPIIDKt00013, Fields: map[string]string{}},
		{Name: "unsettled-orders", Method: http.MethodPost, Path: kiwoom.PathAccount, TRID: kiwoomspecs.KiwoomAPIIDKa10075, Fields: map[string]string{}},
		{Name: "order-executions", Method: http.MethodPost, Path: kiwoom.PathAccount, TRID: kiwoomspecs.KiwoomAPIIDKa10076, Fields: map[string]string{}},
		// 차트
		{Name: "daily-chart", Method: http.MethodPost, Path: kiwoom.PathChart, TRID: kiwoomspecs.KiwoomAPIIDKa10081, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "weekly-chart", Method: http.MethodPost, Path: kiwoom.PathChart, TRID: kiwoomspecs.KiwoomAPIIDKa10082, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "monthly-chart", Method: http.MethodPost, Path: kiwoom.PathChart, TRID: kiwoomspecs.KiwoomAPIIDKa10083, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "tick-chart", Method: http.MethodPost, Path: kiwoom.PathChart, TRID: kiwoomspecs.KiwoomAPIIDKa10079, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "investor-chart", Method: http.MethodPost, Path: kiwoom.PathChart, TRID: kiwoomspecs.KiwoomAPIIDKa10060, Fields: map[string]string{"stk_cd": "005930"}},
		// 문서 등록 strict 라우트
		{Name: "documented-stock-agent", Method: http.MethodPost, Path: kiwoom.PathStockInfo, TRID: kiwoomspecs.KiwoomAPIIDKa10002, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "documented-theme-list", Method: http.MethodPost, Path: kiwoom.PathTheme, TRID: kiwoomspecs.KiwoomAPIIDKa90001, Fields: map[string]string{"qry_tp": "0", "date_tp": "10", "flu_pl_amt_tp": "1", "stex_tp": "1"}},
		{Name: "documented-foreign-inst", Method: http.MethodPost, Path: kiwoom.PathForeignInst, TRID: kiwoomspecs.KiwoomAPIIDKa10008, Fields: map[string]string{"stk_cd": "005930"}},
		{Name: "documented-short-sell", Method: http.MethodPost, Path: kiwoom.PathShortSell, TRID: kiwoomspecs.KiwoomAPIIDKa10014, Fields: map[string]string{"stk_cd": "005930", "strt_dt": "20250501", "end_dt": "20250519"}},
		// 주문 (dry-run, 에러 기대)
		{Name: "place-buy-order", Method: http.MethodPost, Path: kiwoom.PathOrder, TRID: kiwoomspecs.KiwoomAPIIDKt10000, Fields: map[string]string{"stk_cd": "000000", "ord_qty": "0", "ord_unpr": "1"}, ExpectError: true},
		{Name: "place-sell-order", Method: http.MethodPost, Path: kiwoom.PathOrder, TRID: kiwoomspecs.KiwoomAPIIDKt10001, Fields: map[string]string{"stk_cd": "000000", "ord_qty": "0", "ord_unpr": "1"}, ExpectError: true},
		{Name: "modify-order", Method: http.MethodPost, Path: kiwoom.PathOrder, TRID: kiwoomspecs.KiwoomAPIIDKt10002, Fields: map[string]string{"orgn_odno": "0000000000", "ord_qty": "1", "ord_unpr": "1000"}, ExpectError: true},
		{Name: "cancel-order", Method: http.MethodPost, Path: kiwoom.PathOrder, TRID: kiwoomspecs.KiwoomAPIIDKt10003, Fields: map[string]string{"orgn_odno": "0000000000", "ord_qty": "1"}, ExpectError: true},
	}
	if !sandbox {
		return cases
	}

	filtered := make([]endpointCase, 0, len(cases))
	for _, tc := range cases {
		if tc.Name == "account-balance" {
			continue
		}
		filtered = append(filtered, tc)
	}
	return filtered
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	return maps.Clone(in)
}

func shorten(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func isKnownKISAccountUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "APBK1271") || strings.Contains(msg, "APBK2102")
}

func printSummary(results []smokeResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Broker != results[j].Broker {
			return results[i].Broker < results[j].Broker
		}
		if results[i].Account != results[j].Account {
			return results[i].Account < results[j].Account
		}
		return results[i].CaseName < results[j].CaseName
	})

	counts := map[string]int{}
	for _, r := range results {
		counts[r.Status]++
	}

	fmt.Println("STATUS COUNTS")
	fmt.Printf("PASS=%d FAIL=%d EXPECTED_ERROR=%d UNEXPECTED_SUCCESS=%d SKIP=%d\n\n",
		counts["PASS"], counts["FAIL"], counts["EXPECTED_ERROR"], counts["UNEXPECTED_SUCCESS"], counts["SKIP"])

	fmt.Println("DETAILS")
	for _, r := range results {
		line := fmt.Sprintf("[%s] %s %s :: %s (%s)", r.Status, r.Broker, r.Account, r.CaseName, r.Duration.Round(time.Millisecond))
		if r.Detail != "" {
			line += " :: " + r.Detail
		}
		fmt.Println(line)
	}
}
