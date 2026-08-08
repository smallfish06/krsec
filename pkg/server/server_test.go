package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smallfish06/krsec/pkg/broker"
)

type fakeBroker struct{}

func (fakeBroker) Name() string { return "FAKE" }

func (fakeBroker) Authenticate(context.Context, broker.Credentials) (*broker.Token, error) {
	return &broker.Token{AccessToken: "t", TokenType: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (fakeBroker) GetQuote(context.Context, string, string) (*broker.Quote, error) {
	return &broker.Quote{}, nil
}

func (fakeBroker) GetOHLCV(context.Context, string, string, broker.OHLCVOpts) ([]broker.OHLCV, error) {
	return []broker.OHLCV{}, nil
}

func (fakeBroker) GetBalance(context.Context, string) (*broker.Balance, error) {
	return &broker.Balance{}, nil
}

func (fakeBroker) GetPositions(context.Context, string) ([]broker.Position, error) {
	return []broker.Position{}, nil
}

func (fakeBroker) PlaceOrder(context.Context, broker.OrderRequest) (*broker.OrderResult, error) {
	return &broker.OrderResult{}, nil
}

func (fakeBroker) CancelOrder(context.Context, string) error { return nil }

func (fakeBroker) ModifyOrder(context.Context, string, broker.ModifyOrderRequest) (*broker.OrderResult, error) {
	return &broker.OrderResult{}, nil
}

type fakeKISProxyBroker struct {
	fakeBroker
	calls int
	resp  map[string]any
}

func (b *fakeKISProxyBroker) Name() string { return broker.NameKIS }

func (b *fakeKISProxyBroker) CallEndpoint(
	context.Context,
	string,
	string,
	string,
	any,
) (any, error) {
	b.calls++
	return b.resp, nil
}

func TestNew_HealthAndAccounts(t *testing.T) {
	t.Parallel()

	s := New(Options{
		Host: "127.0.0.1",
		Port: 18080,
		Accounts: []Account{
			{ID: "12345678-01", Name: "main", Broker: "custom"},
		},
		Brokers: map[string]broker.Broker{
			"12345678-01": fakeBroker{},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	s.App().Mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected health status: %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/accounts", nil)
	rr = httptest.NewRecorder()
	s.App().Mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected accounts status: %d", rr.Code)
	}
	body := rr.Body.Bytes()

	var got struct {
		OK   bool                 `json:"ok"`
		Data []broker.AccountInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal accounts response: %v", err)
	}
	if !got.OK {
		t.Fatalf("expected ok=true, got false body=%s", string(body))
	}
	if len(got.Data) != 1 {
		t.Fatalf("expected one account, got %d", len(got.Data))
	}
	if got.Data[0].ID != "12345678-01" || got.Data[0].Broker != "custom" {
		t.Fatalf("unexpected account row: %+v", got.Data[0])
	}
}

func TestNew_KISProxyCachePolicyCanDisableDefaultCaching(t *testing.T) {
	kisBroker := &fakeKISProxyBroker{
		resp: map[string]any{"rt_cd": "0", "price": "70000"},
	}
	policyCalled := false
	s := New(Options{
		Host: "127.0.0.1",
		Port: 18082,
		Accounts: []Account{
			{ID: "kis-acc", Name: "main", Broker: "kis"},
		},
		Brokers: map[string]broker.Broker{
			"kis-acc": kisBroker,
		},
		KISProxyCache: KISProxyCacheOptions{
			Policy: func(req KISProxyCacheRequest) (time.Duration, bool) {
				policyCalled = true
				if req.Path != "/uapi/domestic-stock/v1/quotations/inquire-price" {
					t.Fatalf("policy path = %q", req.Path)
				}
				if req.TRID != "FHKST01010100" {
					t.Fatalf("policy tr_id = %q", req.TRID)
				}
				if req.AccountID != "kis-acc" {
					t.Fatalf("policy account_id = %q", req.AccountID)
				}
				if got := req.Params["FID_INPUT_ISCD"]; got != "005930" {
					t.Fatalf("policy FID_INPUT_ISCD = %q", got)
				}
				return 0, false
			},
		},
	})

	body := []byte(`{"tr_id":"FHKST01010100","params":{"FID_COND_MRKT_DIV_CODE":"J","FID_INPUT_ISCD":"005930"}}`)
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/kis/domestic-stock/v1/quotations/inquire-price", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		s.App().Mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected KIS proxy status: %d body=%s", rr.Code, rr.Body.String())
		}
	}
	if !policyCalled {
		t.Fatalf("expected custom cache policy to be called")
	}
	if kisBroker.calls != 2 {
		t.Fatalf("broker calls = %d, want 2", kisBroker.calls)
	}
}

func TestNew_OpenAPIEndpoints(t *testing.T) {
	t.Parallel()

	s := New(Options{
		Host: "127.0.0.1",
		Port: 18081,
		Accounts: []Account{
			{ID: "12345678-01", Name: "main", Broker: "custom"},
		},
		Brokers: map[string]broker.Broker{
			"12345678-01": fakeBroker{},
		},
	})

	// In non-Run tests, OpenAPI routes are explicitly registered on the mux.
	s.App().RegisterOpenAPIRoutes(s.App())

	req := httptest.NewRequest(http.MethodGet, "/swagger/openapi.json", nil)
	rr := httptest.NewRecorder()
	s.App().Mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected openapi spec status: %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"openapi":"3.1.0"`) {
		t.Fatalf("unexpected openapi spec body: %s", rr.Body.String())
	}

	var openapi map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &openapi); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	paths, ok := openapi["paths"].(map[string]any)
	if !ok {
		t.Fatalf("openapi paths not found")
	}
	pathItem, ok := paths["/kis/domestic-bond/v1/quotations/inquire-price"].(map[string]any)
	if !ok {
		t.Fatalf("expected static KIS endpoint in openapi spec")
	}
	if _, hasWildcard := paths["/kis/{path...}"]; hasWildcard {
		t.Fatalf("unexpected wildcard KIS endpoint in openapi spec")
	}
	postOp, ok := pathItem["post"].(map[string]any)
	if !ok {
		t.Fatalf("expected POST operation in static KIS endpoint")
	}

	reqBody, ok := postOp["requestBody"].(map[string]any)
	if !ok {
		t.Fatalf("requestBody missing in static KIS endpoint")
	}
	reqContent, ok := reqBody["content"].(map[string]any)
	if !ok {
		t.Fatalf("requestBody.content missing in static KIS endpoint")
	}
	reqAppJSON, ok := reqContent["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("requestBody.content.application/json missing in static KIS endpoint")
	}
	reqSchema, ok := reqAppJSON["schema"].(map[string]any)
	if !ok {
		t.Fatalf("requestBody schema missing in static KIS endpoint")
	}
	if got, _ := reqSchema["$ref"].(string); got != "#/components/schemas/KISDomesticBondV1QuotationsInquirePriceRequest" {
		t.Fatalf("unexpected static request schema ref: %q", got)
	}

	responses, ok := postOp["responses"].(map[string]any)
	if !ok {
		t.Fatalf("responses missing in static KIS endpoint")
	}
	resp200, ok := responses["200"].(map[string]any)
	if !ok {
		t.Fatalf("responses.200 missing in static KIS endpoint")
	}
	respContent, ok := resp200["content"].(map[string]any)
	if !ok {
		t.Fatalf("responses.200.content missing in static KIS endpoint")
	}
	respAppJSON, ok := respContent["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("responses.200.content.application/json missing in static KIS endpoint")
	}
	respSchema, ok := respAppJSON["schema"].(map[string]any)
	if !ok {
		t.Fatalf("responses.200 schema missing in static KIS endpoint")
	}
	if got, _ := respSchema["$ref"].(string); got != "#/components/schemas/KISDomesticBondV1QuotationsInquirePrice" {
		t.Fatalf("unexpected static response schema ref: %q", got)
	}

	kiwoomPathItem, ok := paths["/kiwoom/dostk/stkinfo/ka10001"].(map[string]any)
	if !ok {
		t.Fatalf("expected static Kiwoom endpoint in openapi spec")
	}
	kiwoomPost, ok := kiwoomPathItem["post"].(map[string]any)
	if !ok {
		t.Fatalf("expected POST operation in static Kiwoom endpoint")
	}
	kiwoomReqBody, ok := kiwoomPost["requestBody"].(map[string]any)
	if !ok {
		t.Fatalf("requestBody missing in static Kiwoom endpoint")
	}
	kiwoomReqContent, ok := kiwoomReqBody["content"].(map[string]any)
	if !ok {
		t.Fatalf("requestBody.content missing in static Kiwoom endpoint")
	}
	kiwoomReqAppJSON, ok := kiwoomReqContent["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("requestBody.content.application/json missing in static Kiwoom endpoint")
	}
	kiwoomReqSchema, ok := kiwoomReqAppJSON["schema"].(map[string]any)
	if !ok {
		t.Fatalf("requestBody schema missing in static Kiwoom endpoint")
	}
	if got, _ := kiwoomReqSchema["$ref"].(string); got != "#/components/schemas/KiwoomApiDostkStkinfoKa10001Request" {
		t.Fatalf("unexpected static Kiwoom request schema ref: %q", got)
	}
	kiwoomResponses, ok := kiwoomPost["responses"].(map[string]any)
	if !ok {
		t.Fatalf("responses missing in static Kiwoom endpoint")
	}
	kiwoomResp200, ok := kiwoomResponses["200"].(map[string]any)
	if !ok {
		t.Fatalf("responses.200 missing in static Kiwoom endpoint")
	}
	kiwoomRespContent, ok := kiwoomResp200["content"].(map[string]any)
	if !ok {
		t.Fatalf("responses.200.content missing in static Kiwoom endpoint")
	}
	kiwoomRespAppJSON, ok := kiwoomRespContent["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("responses.200.content.application/json missing in static Kiwoom endpoint")
	}
	kiwoomRespSchema, ok := kiwoomRespAppJSON["schema"].(map[string]any)
	if !ok {
		t.Fatalf("responses.200 schema missing in static Kiwoom endpoint")
	}
	if got, _ := kiwoomRespSchema["$ref"].(string); got != "#/components/schemas/KiwoomApiDostkStkinfoKa10001Response" {
		t.Fatalf("unexpected static Kiwoom response schema ref: %q", got)
	}
	components, ok := openapi["components"].(map[string]any)
	if !ok {
		t.Fatalf("openapi components not found")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatalf("openapi components.schemas not found")
	}
	mrkcondSchema, ok := schemas["KiwoomApiDostkMrkcondKa10066Response"].(map[string]any)
	if !ok {
		t.Fatalf("KiwoomApiDostkMrkcondKa10066Response schema not found")
	}
	mrkcondProps, ok := mrkcondSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("KiwoomApiDostkMrkcondKa10066Response.properties not found")
	}
	listProp, ok := mrkcondProps["opaf_invsr_trde"].(map[string]any)
	if !ok {
		t.Fatalf("KiwoomApiDostkMrkcondKa10066Response.opaf_invsr_trde not found")
	}
	listItems, ok := listProp["items"].(map[string]any)
	if !ok {
		t.Fatalf("KiwoomApiDostkMrkcondKa10066Response.opaf_invsr_trde.items not found")
	}
	// Array item schemas are emitted as named $refs; resolve before inspecting.
	if ref, _ := listItems["$ref"].(string); ref != "" {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		listItems, ok = schemas[name].(map[string]any)
		if !ok {
			t.Fatalf("KiwoomApiDostkMrkcondKa10066Response item schema %q not found", name)
		}
	}
	itemProps, ok := listItems["properties"].(map[string]any)
	if !ok {
		t.Fatalf("KiwoomApiDostkMrkcondKa10066Response item properties not found")
	}
	for key := range itemProps {
		if strings.HasPrefix(key, "- ") {
			t.Fatalf("unexpected hyphen-prefixed Kiwoom response field key: %q", key)
		}
	}
	if _, hasWildcard := paths["/kiwoom/{path...}"]; !hasWildcard {
		t.Fatalf("expected wildcard Kiwoom endpoint in openapi spec")
	}

	req = httptest.NewRequest(http.MethodGet, "/swagger/", nil)
	rr = httptest.NewRecorder()
	s.App().Mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected swagger ui status: %d body=%s", rr.Code, rr.Body.String())
	}
}
