package server

import (
	"errors"
	"strings"

	"github.com/go-playground/validator/v10"
)

var proxyRequestValidator = validator.New()

func validateKISProxyRequest(req *kisProxyRequest) error {
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	req.TRID = strings.TrimSpace(req.TRID)

	if err := proxyRequestValidator.Struct(struct {
		Method string `validate:"omitempty,oneof=GET POST PUT DELETE PATCH"`
	}{
		Method: req.Method,
	}); err != nil {
		return proxyRequestValidationError(err, map[string]string{
			"Method.oneof": "unsupported method",
		})
	}
	return nil
}

func validateKiwoomProxyRequest(req *kiwoomProxyRequest) error {
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	req.APIID = strings.TrimSpace(req.APIID)

	if err := proxyRequestValidator.Struct(struct {
		Method string `validate:"omitempty,oneof=GET POST PUT DELETE PATCH"`
		APIID  string `validate:"required"`
	}{
		Method: req.Method,
		APIID:  req.APIID,
	}); err != nil {
		return proxyRequestValidationError(err, map[string]string{
			"APIID.required": "api_id is required",
			"Method.oneof":   "unsupported method",
		})
	}
	return nil
}

func validateLSProxyRequest(req *lsProxyRequest) error {
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	req.TRCD = strings.TrimSpace(req.TRCD)

	if err := proxyRequestValidator.Struct(struct {
		Method string `validate:"omitempty,oneof=GET POST PUT DELETE PATCH"`
		TRCD   string `validate:"required"`
	}{
		Method: req.Method,
		TRCD:   req.TRCD,
	}); err != nil {
		return proxyRequestValidationError(err, map[string]string{
			"TRCD.required": "tr_cd is required",
			"Method.oneof":  "unsupported method",
		})
	}
	return nil
}

func proxyRequestValidationError(err error, messages map[string]string) error {
	var invalidValidationErr *validator.InvalidValidationError
	if errors.As(err, &invalidValidationErr) {
		return errors.New("invalid request body")
	}

	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		return errors.New("invalid request body")
	}

	for _, validationErr := range validationErrs {
		key := validationErr.Field() + "." + validationErr.Tag()
		if msg, ok := messages[key]; ok {
			return errors.New(msg)
		}
	}

	return errors.New("invalid request body")
}
