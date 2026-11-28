package server

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
)

const ambiguousAccountIDError = "account_id is ambiguous; use full account_id"

// proxyAccountSelector merges the account_id from a raw proxy request body
// with the documented ?account_id= query parameter. Two different values are
// rejected instead of silently preferring one: the selector decides which
// account a raw order endpoint acts on.
func proxyAccountSelector(bodyValue, queryValue string) (string, error) {
	bodyValue = strings.TrimSpace(bodyValue)
	queryValue = strings.TrimSpace(queryValue)
	switch {
	case bodyValue == "":
		return queryValue, nil
	case queryValue == "" || sameAccountID(bodyValue, queryValue):
		return bodyValue, nil
	default:
		return "", errors.New("account_id in body does not match account_id in query")
	}
}

func normalizeBrokerCode(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case broker.CodeKIS, strings.ToLower(broker.NameKIS):
		return broker.CodeKIS
	case broker.CodeKiwoom, strings.ToLower(broker.NameKiwoom):
		return broker.CodeKiwoom
	case broker.CodeLS, strings.ToLower(broker.NameLS):
		return broker.CodeLS
	case broker.CodeToss, strings.ToLower(broker.NameToss):
		return broker.CodeToss
	default:
		return ""
	}
}

func normalizeAccountIDAlias(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if before, ok := strings.CutSuffix(accountID, "-01"); ok {
		return before
	}
	return accountID
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func kisAccountBase(accountID string) (string, bool) {
	accountID = strings.TrimSpace(accountID)
	switch {
	case len(accountID) == 8 && isDigits(accountID):
		return accountID, true
	case len(accountID) == 11 && accountID[8] == '-' && isDigits(accountID[:8]) && isDigits(accountID[9:]):
		return accountID[:8], true
	default:
		return "", false
	}
}

func sameAccountID(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	if baseA, okA := kisAccountBase(a); okA {
		if baseB, okB := kisAccountBase(b); okB {
			return baseA == baseB
		}
	}
	return normalizeAccountIDAlias(a) == normalizeAccountIDAlias(b)
}

func (s *Server) resolveBrokerByAccountID(accountID string) (broker.Broker, int, string) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, http.StatusBadRequest, "account_id is required"
	}

	if brk, ok := s.brokers[accountID]; ok {
		return brk, 0, ""
	}

	candidates := s.findBrokerAccountCandidates(accountID)
	switch len(candidates) {
	case 0:
		return nil, http.StatusNotFound, "account not found"
	case 1:
		return s.brokers[candidates[0]], 0, ""
	default:
		return nil, http.StatusBadRequest, ambiguousAccountIDError
	}
}

func (s *Server) getBrokerStrict(accountID string) (broker.Broker, bool) {
	brk, status, _ := s.resolveBrokerByAccountID(accountID)
	return brk, status == 0
}

func (s *Server) findBrokerAccountCandidates(accountID string) []string {
	var matches []string
	for _, id := range s.orderedAccountIDs() {
		if sameAccountID(id, accountID) {
			matches = append(matches, id)
		}
	}
	return matches
}

// orderedAccountIDs lists every account that has a broker bound, in configured
// order followed by any unconfigured ones sorted by ID. Handlers iterate this
// instead of the brokers map so fallbacks are deterministic.
func (s *Server) orderedAccountIDs() []string {
	ids := make([]string, 0, len(s.brokers))
	seen := make(map[string]struct{}, len(s.brokers))
	for _, acc := range s.accounts {
		id := strings.TrimSpace(acc.AccountID)
		if s.brokers[id] == nil {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	configured := len(ids)
	for id, brk := range s.brokers {
		if _, dup := seen[id]; dup || brk == nil {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids[configured:])
	return ids
}

func (s *Server) orderedBrokers() []broker.Broker {
	ids := s.orderedAccountIDs()
	out := make([]broker.Broker, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.brokers[id])
	}
	return out
}

// marketDataBroker picks the broker for a quote or candle request: the
// selected account when account_id is given, otherwise the first available.
func (s *Server) marketDataBroker(accountID string) (broker.Broker, int, string) {
	if accountID = strings.TrimSpace(accountID); accountID != "" {
		return s.resolveBrokerByAccountID(accountID)
	}
	if ids := s.orderedAccountIDs(); len(ids) > 0 {
		return s.brokers[ids[0]], 0, ""
	}
	return nil, http.StatusServiceUnavailable, "no broker available"
}

func (s *Server) accountConfig(accountID string) (config.AccountConfig, bool) {
	for _, acc := range s.accounts {
		if strings.TrimSpace(acc.AccountID) == accountID {
			return acc, true
		}
	}
	return config.AccountConfig{}, false
}

// resolveAuthAccount picks the account POST /auth/token acts on: the first
// account of the requested broker, preferring one whose sandbox flag matches.
// The returned config is zero for brokers supplied without an account entry.
func (s *Server) resolveAuthAccount(requestedBroker string, sandbox bool) (broker.Broker, config.AccountConfig, int, string) {
	brokerCode := normalizeBrokerCode(requestedBroker)
	if strings.TrimSpace(requestedBroker) != "" && brokerCode == "" {
		return nil, config.AccountConfig{}, http.StatusBadRequest, "unsupported broker"
	}

	ids := s.orderedAccountIDs()
	if brokerCode == "" {
		if len(ids) == 0 {
			return nil, config.AccountConfig{}, http.StatusServiceUnavailable, "no broker available"
		}
		acc, _ := s.accountConfig(ids[0])
		return s.brokers[ids[0]], acc, 0, ""
	}

	fallbackID := ""
	for _, id := range ids {
		acc, configured := s.accountConfig(id)
		if configured {
			if normalizeBrokerCode(acc.Broker) != brokerCode {
				continue
			}
			if acc.Sandbox == sandbox {
				return s.brokers[id], acc, 0, ""
			}
		} else if normalizeBrokerCode(s.brokers[id].Name()) != brokerCode {
			continue
		}
		if fallbackID == "" {
			fallbackID = id
		}
	}
	if fallbackID != "" {
		acc, _ := s.accountConfig(fallbackID)
		return s.brokers[fallbackID], acc, 0, ""
	}
	return nil, config.AccountConfig{}, http.StatusServiceUnavailable, "no " + strings.ToUpper(brokerCode) + " account available"
}
