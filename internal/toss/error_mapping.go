package toss

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
)

type errorEnvelope struct {
	Error struct {
		RequestID string         `json:"requestId"`
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		Data      map[string]any `json:"data"`
	} `json:"error"`
}

type oauthErrorEnvelope struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func mapUpstreamError(status int, body []byte) error {
	code, message := parseError(body)
	sentinel := classifyUpstream(status, code, message)
	text := strings.TrimSpace(string(body))
	if message != "" {
		text = message
	}
	if code != "" {
		return fmt.Errorf("%w: Toss %s HTTP %d: %s", sentinel, code, status, text)
	}
	return fmt.Errorf("%w: Toss HTTP %d: %s", sentinel, status, text)
}

func mapOAuthError(status int, body []byte) error {
	code, message := parseOAuthError(body)
	sentinel := classifyUpstream(status, code, message)
	if code == "invalid_client" {
		sentinel = broker.ErrInvalidCredentials
	}
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if code != "" {
		return fmt.Errorf("%w: Toss auth %s HTTP %d: %s", sentinel, code, status, message)
	}
	return fmt.Errorf("%w: Toss auth HTTP %d: %s", sentinel, status, message)
}

func classifyUpstream(status int, code, message string) error {
	code = strings.ToLower(strings.TrimSpace(code))
	msg := strings.ToLower(strings.TrimSpace(message))

	switch {
	case status == 429:
		return broker.ErrRateLimitExceeded
	case status == 401 || code == "invalid-token" || strings.Contains(msg, "token"):
		return broker.ErrUnauthorized
	case code == "invalid_client":
		return broker.ErrInvalidCredentials
	case code == "order-not-found":
		return broker.ErrOrderNotFound
	case code == "account-not-found":
		return broker.ErrUpstreamBadRequest
	case strings.Contains(code, "not-found") || status == 404:
		return broker.ErrInstrumentNotFound
	case strings.Contains(code, "insufficient"):
		return broker.ErrInsufficientBalance
	case strings.Contains(code, "invalid") || status == 400 || status == 422:
		return broker.ErrUpstreamBadRequest
	case status >= 500:
		return broker.ErrServerError
	default:
		return broker.ErrServerError
	}
}

func parseError(body []byte) (string, string) {
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err == nil {
		return strings.TrimSpace(env.Error.Code), strings.TrimSpace(env.Error.Message)
	}
	return "", ""
}

func parseOAuthError(body []byte) (string, string) {
	var env oauthErrorEnvelope
	if err := json.Unmarshal(body, &env); err == nil {
		return strings.TrimSpace(env.Error), strings.TrimSpace(env.ErrorDescription)
	}
	return "", ""
}
