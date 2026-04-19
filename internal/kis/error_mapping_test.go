package kis

import (
	"errors"
	"testing"

	"github.com/smallfish06/krsec/pkg/broker"
)

func TestClassifyUpstream(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{
			// KIS returns HTTP 500 + msg_cd=EGW00201 for per-second TPS overrun.
			// Must promote to rate-limit so callers map it to 429.
			name:   "tps overrun masked as 500",
			status: 500,
			body:   `{"rt_cd":"1","msg_cd":"EGW00201","msg1":"초당 거래건수를 초과하였습니다."}`,
			want:   broker.ErrRateLimitExceeded,
		},
		{
			name:   "http 429 without msg_cd",
			status: 429,
			body:   ``,
			want:   broker.ErrRateLimitExceeded,
		},
		{
			name:   "generic 500",
			status: 500,
			body:   `{"rt_cd":"1","msg_cd":"EGW00999","msg1":"boom"}`,
			want:   broker.ErrServerError,
		},
		{
			name:   "4xx",
			status: 400,
			body:   `{"rt_cd":"1","msg_cd":"EGW00123","msg1":"invalid request"}`,
			want:   broker.ErrUpstreamBadRequest,
		},
		{
			name:   "non-json 500",
			status: 500,
			body:   `upstream exploded`,
			want:   broker.ErrServerError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyUpstream(tc.status, []byte(tc.body))
			if !errors.Is(got, tc.want) {
				t.Fatalf("classifyUpstream(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
			}
		})
	}
}
