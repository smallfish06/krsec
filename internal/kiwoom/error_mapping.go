package kiwoom

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
)

func wrapCallError(apiID string, code int, msg string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "request failed"
	}
	if mapped := mapKiwoomError(code, msg); mapped != nil {
		return fmt.Errorf("%w: kiwoom api %s failed (%d): %s", mapped, apiID, code, msg)
	}
	return fmt.Errorf("kiwoom api %s failed (%d): %s", apiID, code, msg)
}

func wrapAuthError(code int, msg string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "authentication failed"
	}
	if mapped := mapKiwoomError(code, msg); mapped != nil {
		if errors.Is(mapped, broker.ErrInvalidCredentials) || errors.Is(mapped, broker.ErrUnauthorized) {
			return fmt.Errorf("%w: %s", mapped, msg)
		}
		return fmt.Errorf("%w: auth failed (%d): %s", mapped, code, msg)
	}
	return fmt.Errorf("auth failed (%d): %s", code, msg)
}

func mapKiwoomError(code int, msg string) error {
	m := strings.ToLower(strings.TrimSpace(msg))

	switch {
	case strings.Contains(m, "appkey"),
		strings.Contains(m, "secret"),
		strings.Contains(m, "credential"),
		strings.Contains(m, "자격"),
		strings.Contains(m, "키가 올바르지"),
		strings.Contains(m, "유효하지 않은 키"):
		return broker.ErrInvalidCredentials
	case strings.Contains(m, "unauthorized"),
		strings.Contains(m, "forbidden"),
		strings.Contains(m, "token"),
		strings.Contains(m, "토큰"),
		strings.Contains(m, "인증"),
		strings.Contains(m, "권한"),
		strings.Contains(m, "만료"):
		return broker.ErrUnauthorized
	case strings.Contains(m, "insufficient"),
		strings.Contains(m, "잔고부족"),
		strings.Contains(m, "예수금 부족"),
		strings.Contains(m, "주문가능수량 부족"),
		strings.Contains(m, "주문가능금액 부족"):
		return broker.ErrInsufficientBalance
	case strings.Contains(m, "주문번호") && (strings.Contains(m, "없") || strings.Contains(m, "not found") || strings.Contains(m, "존재")),
		strings.Contains(m, "원주문번호") && (strings.Contains(m, "없") || strings.Contains(m, "not found") || strings.Contains(m, "존재")):
		return broker.ErrOrderNotFound
	case strings.Contains(m, "종목") && (strings.Contains(m, "없") || strings.Contains(m, "유효하지") || strings.Contains(m, "오류") || strings.Contains(m, "not found") || strings.Contains(m, "invalid")):
		return broker.ErrInvalidSymbol
	case strings.Contains(m, "symbol") && (strings.Contains(m, "invalid") || strings.Contains(m, "not found")):
		return broker.ErrInvalidSymbol
	case strings.Contains(m, "시장"),
		strings.Contains(m, "거래소"),
		strings.Contains(m, "market"):
		return broker.ErrInvalidMarket
	case strings.Contains(m, "주문수량"),
		strings.Contains(m, "주문가격"),
		strings.Contains(m, "입력"),
		strings.Contains(m, "파라미터"),
		strings.Contains(m, "필수"),
		strings.Contains(m, "invalid request"):
		return broker.ErrInvalidOrderRequest
	}

	switch code {
	case 401, 403:
		return broker.ErrUnauthorized
	case 404:
		return broker.ErrOrderNotFound
	case 429:
		return broker.ErrRateLimitExceeded
	}
	if code >= 500 {
		return broker.ErrServerError
	}
	return nil
}

func parseErrorPayload(body []byte) (int, string, bool) {
	if len(bytes.TrimSpace(body)) == 0 {
		return 0, "", false
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return 0, "", false
	}
	code := parseReturnCode(obj["return_code"])
	msg := strings.TrimSpace(toString(obj["return_msg"]))
	if code == 0 && msg == "" {
		return 0, "", false
	}
	return code, msg, true
}
