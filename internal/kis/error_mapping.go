package kis

import (
	"encoding/json"
	"fmt"

	"github.com/smallfish06/krsec/pkg/broker"
)

// mapUpstreamError classifies a failed KIS upstream response (HTTP >= 400)
// into a broker sentinel error, consulting both the HTTP status code and
// the vendor-specific msg_cd in the response body.
//
// KIS does not always use HTTP status codes the way callers expect —
// e.g. per-second TPS overruns come back as HTTP 500 with
// msg_cd=EGW00201 rather than HTTP 429 — so payload inspection is
// required for correct classification.
func mapUpstreamError(status int, body []byte) error {
	sentinel := classifyUpstream(status, body)
	return fmt.Errorf("%w: HTTP %d: %s", sentinel, status, string(body))
}

func classifyUpstream(status int, body []byte) error {
	if parseMsgCode(body) == "EGW00201" {
		return broker.ErrRateLimitExceeded
	}

	switch {
	case status == 429:
		return broker.ErrRateLimitExceeded
	case status >= 500:
		return broker.ErrServerError
	default:
		return broker.ErrUpstreamBadRequest
	}
}

func parseMsgCode(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var env struct {
		MsgCode string `json:"msg_cd"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	return env.MsgCode
}
