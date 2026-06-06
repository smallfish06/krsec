package specs

import "testing"

func TestLookupDocumentedEndpointSpec_KnownRESTAndWebSocketTRs(t *testing.T) {
	t.Parallel()

	quote, ok := LookupDocumentedEndpointSpec("/stock/market-data", "t1102")
	if !ok {
		t.Fatal("missing documented t1102 spec")
	}
	if quote.Protocol != ProtocolREST || quote.Method != "POST" {
		t.Fatalf("unexpected t1102 protocol/method: %+v", quote)
	}
	if !hasRequiredRequestField(quote, "t1102InBlock") ||
		!hasRequiredRequestField(quote, "shcode") ||
		!hasRequiredRequestField(quote, "exchgubun") {
		t.Fatalf("t1102 required fields missing: %+v", quote.RequestFields)
	}

	overseas, ok := LookupDocumentedEndpointSpec("overseas-stock/market-data", "g3101")
	if !ok {
		t.Fatal("missing documented g3101 spec")
	}
	if overseas.Protocol != ProtocolREST {
		t.Fatalf("unexpected g3101 protocol: %+v", overseas)
	}

	realtime, ok := LookupDocumentedEndpointSpec("/websocket/overseas-stock", "GSC")
	if !ok {
		t.Fatal("missing documented GSC spec")
	}
	if realtime.Protocol != ProtocolWebSocket {
		t.Fatalf("unexpected GSC protocol: %+v", realtime)
	}
}

func TestDocumentedEndpointSpecCounts(t *testing.T) {
	t.Parallel()

	if got := DocumentedEndpointSpecCount(); got < 300 {
		t.Fatalf("DocumentedEndpointSpecCount = %d, want >= 300", got)
	}
	if got := DocumentedRESTEndpointSpecCount(); got < 200 {
		t.Fatalf("DocumentedRESTEndpointSpecCount = %d, want >= 200", got)
	}
	if got := DocumentedWebSocketEndpointSpecCount(); got < 100 {
		t.Fatalf("DocumentedWebSocketEndpointSpecCount = %d, want >= 100", got)
	}
}

func hasRequiredRequestField(spec LSEndpointSpec, code string) bool {
	for _, field := range spec.RequestFields {
		if field.Code == code && field.Required {
			return true
		}
	}
	return false
}
