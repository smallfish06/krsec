package kis

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	kisspecs "github.com/smallfish06/krsec/pkg/kis/specs"
)

func TestDocumentedEndpointResponseFactoryCoverage(t *testing.T) {
	t.Parallel()

	if got, want := kisspecs.DocumentedEndpointResponseFactoryCount(), len(kisspecs.DocumentedKISEndpointSpecs); got != want {
		t.Fatalf("factory count = %d, spec count = %d", got, want)
	}
}

func TestNewDocumentedEndpointResponse_KnownAndUnknown(t *testing.T) {
	t.Parallel()

	got := kisspecs.NewDocumentedEndpointResponse("/uapi/domestic-stock/v1/quotations/inquire-price")
	if got == nil {
		t.Fatal("expected typed response for known path")
	}
	if _, ok := got.(*kisspecs.KISDomesticStockV1QuotationsInquirePrice); !ok {
		t.Fatalf("unexpected type: %T", got)
	}

	if unk := kisspecs.NewDocumentedEndpointResponse("/uapi/unknown/path"); unk != nil {
		t.Fatalf("expected nil for unknown path, got %T", unk)
	}
}

func TestCallDocumentedEndpointInto_TypedResponseCheck(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uapi/domestic-stock/v1/quotations/chk-holiday" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"rt_cd":"0","msg_cd":"00000","msg1":"ok","output":[{"bass_dt":"20260302","opnd_yn":"Y"}]}`))
	}))
	defer ts.Close()

	c := newAuthedTestClient(ts.URL)
	resp := kisspecs.NewDocumentedEndpointResponse("/uapi/domestic-stock/v1/quotations/chk-holiday")
	if resp == nil {
		t.Fatal("typed response factory returned nil")
	}

	if err := c.CallDocumentedEndpointInto(context.Background(), http.MethodGet, "/uapi/domestic-stock/v1/quotations/chk-holiday", "CTCA0903R", map[string]string{
		"BASS_DT": "20260302",
	}, resp); err != nil {
		t.Fatalf("CallDocumentedEndpointInto returned error: %v", err)
	}
}

func TestDocumentedResponseBase_EmptyRtCdTreatedSuccess(t *testing.T) {
	t.Parallel()

	base := &kisspecs.DocumentedResponseBase{}
	if !base.IsSuccess() {
		t.Fatal("expected empty rt_cd to be treated as success")
	}
}
