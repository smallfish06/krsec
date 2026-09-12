package kiwoom

import (
	"context"
	"fmt"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
	kiwoomspecs "github.com/smallfish06/krsec/pkg/kiwoom/specs"
)

type pageContextKey struct{}

type pageCapture struct {
	request  kiwoomspecs.Continuation
	response kiwoomspecs.Continuation
}

// CaptureEndpointPage preserves the existing typed endpoint preparation and
// validation while collecting the headers from its single HTTP request. The
// capture belongs to this invocation and is never stored on the shared client.
func CaptureEndpointPage(ctx context.Context, continuation kiwoomspecs.Continuation, call func(context.Context) (any, error)) (*kiwoomspecs.EndpointPage, error) {
	continuation.ContYN = strings.ToUpper(strings.TrimSpace(continuation.ContYN))
	continuation.NextKey = strings.TrimSpace(continuation.NextKey)
	if continuation.ContYN != "" && continuation.ContYN != "N" && continuation.ContYN != "Y" {
		return nil, fmt.Errorf("%w: cont-yn must be Y or N", broker.ErrInvalidOrderRequest)
	}
	if (continuation.ContYN == "Y") != (continuation.NextKey != "") {
		return nil, fmt.Errorf("%w: cont-yn=Y and next-key must be supplied together", broker.ErrInvalidOrderRequest)
	}
	if strings.ContainsAny(continuation.NextKey, "\r\n") {
		return nil, fmt.Errorf("%w: invalid next-key", broker.ErrInvalidOrderRequest)
	}
	capture := &pageCapture{request: continuation}
	data, err := call(context.WithValue(ctx, pageContextKey{}, capture))
	if err != nil {
		return nil, err
	}
	return &kiwoomspecs.EndpointPage{Data: data, Continuation: capture.response}, nil
}
