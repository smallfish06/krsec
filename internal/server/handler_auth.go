package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
)

// AuthTokenRequest selects the account whose broker token to issue.
// Credentials are optional for configured accounts; when present they must
// match the configured app key and secret.
type AuthTokenRequest struct {
	Broker      string             `json:"broker"`
	Credentials broker.Credentials `json:"credentials"`
	Sandbox     bool               `json:"sandbox"`
}

func (s *Server) handleAuthToken(c fuego.ContextWithBody[AuthTokenRequest]) (Response, error) {
	req, err := c.Body()
	if err != nil {
		return respond(c, http.StatusBadRequest, Response{Error: "invalid request body"})
	}

	brk, account, status, reason := s.resolveAuthAccount(req.Broker, req.Sandbox)
	if brk == nil {
		return respond(c, status, Response{Error: reason})
	}

	creds, ok := authCredentials(req.Credentials, account)
	if !ok {
		return respond(c, http.StatusUnauthorized, Response{
			Error:  "credentials do not match the configured account",
			Broker: brk.Name(),
		})
	}

	token, err := brk.Authenticate(c.Context(), creds)
	if err != nil {
		return respond(c, statusFromBrokerError(err, http.StatusUnauthorized), Response{Error: err.Error()})
	}

	return respond(c, http.StatusOK, Response{
		OK:     true,
		Data:   token,
		Broker: brk.Name(),
	})
}

// authCredentials returns the credentials to authenticate account with.
//
// Authenticate stores the credentials on the adapter shared by every request
// for that account, so a configured account only ever authenticates with its
// own key pair: passing caller-supplied ones through would let one HTTP request
// rebind the account to a different app key. Brokers supplied without
// configured credentials (pkg/server embedders) receive the request's as-is.
func authCredentials(requested broker.Credentials, account config.AccountConfig) (broker.Credentials, bool) {
	configured := broker.Credentials{
		AppKey:    strings.TrimSpace(account.AppKey),
		AppSecret: strings.TrimSpace(account.AppSecret),
	}
	if configured.AppKey == "" && configured.AppSecret == "" {
		return requested, true
	}

	requested.AppKey = strings.TrimSpace(requested.AppKey)
	requested.AppSecret = strings.TrimSpace(requested.AppSecret)
	if requested.AppKey == "" && requested.AppSecret == "" {
		return configured, true
	}
	keyOK := subtle.ConstantTimeCompare([]byte(requested.AppKey), []byte(configured.AppKey))
	secretOK := subtle.ConstantTimeCompare([]byte(requested.AppSecret), []byte(configured.AppSecret))
	return configured, keyOK&secretOK == 1
}
