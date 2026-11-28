package server

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/go-fuego/fuego"

	"github.com/smallfish06/krsec/pkg/broker"
)

func (s *Server) handleListAccounts(c fuego.ContextNoBody) (Response, error) {
	accounts := make([]broker.AccountInfo, 0, len(s.accounts))
	for _, account := range s.accounts {
		accounts = append(accounts, broker.AccountInfo{
			ID:     account.AccountID,
			Name:   account.Name,
			Broker: account.Broker,
		})
	}

	return respond(c, http.StatusOK, Response{
		OK:   true,
		Data: accounts,
	})
}

// summaryFields are the balance metrics /accounts/summary adds up. An account
// that cannot report one of them makes the total meaningless.
var summaryFields = []string{"total_assets", "cash", "profit_loss"}

// handleAccountsSummary totals balances across every configured account. It
// refuses to publish a partial total: one missing account would read as a
// drop in assets.
func (s *Server) handleAccountsSummary(c fuego.ContextNoBody) (Response, error) {
	ctx := c.Context()

	balances := make([]*broker.Balance, len(s.accounts))
	errs := make([]error, len(s.accounts))
	var wg sync.WaitGroup
	for i, account := range s.accounts {
		wg.Go(func() {
			balances[i], errs[i] = s.summaryBalance(ctx, account.AccountID)
		})
	}
	wg.Wait()

	summary := broker.AccountSummary{Accounts: make([]broker.Balance, 0, len(s.accounts))}
	var incomplete []string
	for i, account := range s.accounts {
		if errs[i] != nil {
			s.logger.Warn("account summary: balance unavailable", "account_id", account.AccountID, "error", errs[i])
			incomplete = append(incomplete, account.AccountID)
			continue
		}
		summary.Accounts = append(summary.Accounts, *balances[i])
		summary.TotalAssets += balances[i].TotalAssets
		summary.TotalCash += balances[i].Cash
		summary.TotalProfitLoss += balances[i].ProfitLoss
	}

	switch {
	case len(incomplete) == 0:
		return respond(c, http.StatusOK, Response{OK: true, Data: summary})
	case len(summary.Accounts) == 0:
		return respond(c, http.StatusServiceUnavailable, Response{
			Error: "failed to retrieve balances from all accounts",
		})
	default:
		return respond(c, http.StatusServiceUnavailable, Response{
			Error: "cannot aggregate balances: accounts unavailable or incomplete: " + strings.Join(incomplete, ", "),
		})
	}
}

// summaryBalance fetches one account's balance and rejects results that
// cannot take part in a total. It runs on its own goroutine, where a panic in
// a broker implementation would end the process, so a panic is reported as
// that account's error.
func (s *Server) summaryBalance(ctx context.Context, accountID string) (balance *broker.Balance, err error) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("account summary: broker panic", "account_id", accountID, "panic", r)
			balance, err = nil, errors.New("broker panicked while fetching balance")
		}
	}()

	brk, status, reason := s.resolveBrokerByAccountID(accountID)
	if status != 0 || brk == nil {
		return nil, errors.New(reason)
	}
	balance, err = brk.GetBalance(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if balance == nil {
		return nil, errors.New("broker returned no balance")
	}
	for _, field := range summaryFields {
		if slices.Contains(balance.UnavailableFields, field) {
			return nil, errors.New("broker cannot report " + field)
		}
	}
	return balance, nil
}
