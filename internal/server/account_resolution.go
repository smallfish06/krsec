package server

import (
	"net/http"
	"sort"
	"strings"

	"github.com/smallfish06/krsec/pkg/broker"
)

const ambiguousAccountIDError = "account_id is ambiguous; use full account_id"

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
	matches := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)

	// Prefer configured account order for deterministic matching.
	for _, acc := range s.accounts {
		id := strings.TrimSpace(acc.AccountID)
		if id == "" {
			continue
		}
		if _, ok := s.brokers[id]; !ok {
			continue
		}
		if !sameAccountID(id, accountID) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		matches = append(matches, id)
	}

	extra := make([]string, 0, len(s.brokers))
	for id := range s.brokers {
		if _, ok := seen[id]; ok {
			continue
		}
		if sameAccountID(id, accountID) {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	matches = append(matches, extra...)

	return matches
}

func (s *Server) resolveAuthBroker(requestedBroker string, sandbox bool) (broker.Broker, int, string) {
	brokerCode := normalizeBrokerCode(requestedBroker)
	if strings.TrimSpace(requestedBroker) != "" && brokerCode == "" {
		return nil, http.StatusBadRequest, "unsupported broker"
	}

	if brokerCode == "" {
		brk := s.getFirstBroker()
		if brk == nil {
			return nil, http.StatusServiceUnavailable, "no broker available"
		}
		return brk, 0, ""
	}

	var fallback broker.Broker
	for _, acc := range s.accounts {
		if normalizeBrokerCode(acc.Broker) != brokerCode {
			continue
		}
		brk, status, _ := s.resolveBrokerByAccountID(acc.AccountID)
		if status != 0 {
			continue
		}
		if acc.Sandbox == sandbox {
			return brk, 0, ""
		}
		if fallback == nil {
			fallback = brk
		}
	}

	if fallback != nil {
		return fallback, 0, ""
	}

	ids := make([]string, 0, len(s.brokers))
	for id := range s.brokers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		brk := s.brokers[id]
		if normalizeBrokerCode(brk.Name()) == brokerCode {
			return brk, 0, ""
		}
	}

	return nil, http.StatusServiceUnavailable, "no " + strings.ToUpper(brokerCode) + " account available"
}
