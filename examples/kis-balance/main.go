// Example: fetch a KIS account balance through the public adapter.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	pkgadapter "github.com/smallfish06/krsec/pkg/adapter"
	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
	"github.com/smallfish06/krsec/pkg/kis"
)

type result struct {
	AccountID   string            `json:"account_id"`
	AccountName string            `json:"account_name"`
	Broker      string            `json:"broker"`
	Sandbox     bool              `json:"sandbox"`
	Balance     *broker.Balance   `json:"balance,omitempty"`
	Positions   []broker.Position `json:"positions,omitempty"`
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	accountSelector := flag.String("account", "", "Account ID or account name (optional, default first account)")
	withPositions := flag.Bool("positions", true, "Include positions in output")
	timeout := flag.Duration("timeout", 20*time.Second, "Request timeout")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	account, err := selectAccount(cfg, *accountSelector)
	if err != nil {
		slog.Error("select account", "error", err)
		os.Exit(1)
	}

	tokenManager := kis.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	adapter := kis.NewAdapterWithOptions(account.Sandbox, account.AccountID, pkgadapter.Options{
		TokenManager:    tokenManager,
		OrderContextDir: cfg.Storage.OrderContextDir,
	})
	creds := broker.Credentials{
		AppKey:    account.AppKey,
		AppSecret: account.AppSecret,
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)

	if _, err := adapter.Authenticate(ctx, creds); err != nil {
		cancel()
		slog.Error("authenticate", "error", err)
		os.Exit(1)
	}

	bal, err := adapter.GetBalance(ctx, account.AccountID)
	if err != nil {
		cancel()
		slog.Error("get balance", "error", err)
		os.Exit(1)
	}

	out := result{
		AccountID:   account.AccountID,
		AccountName: account.Name,
		Broker:      account.Broker,
		Sandbox:     account.Sandbox,
		Balance:     bal,
	}

	if *withPositions {
		pos, err := adapter.GetPositions(ctx, account.AccountID)
		if err != nil {
			cancel()
			slog.Error("get positions", "error", err)
			os.Exit(1)
		}
		out.Positions = pos
	}
	cancel()

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		slog.Error("marshal output", "error", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func selectAccount(cfg *config.Config, selector string) (config.AccountConfig, error) {
	if len(cfg.Accounts) == 0 {
		return config.AccountConfig{}, fmt.Errorf("no accounts configured")
	}
	if selector == "" {
		return cfg.Accounts[0], nil
	}
	for _, acc := range cfg.Accounts {
		if acc.AccountID == selector || acc.Name == selector {
			return acc, nil
		}
	}
	return config.AccountConfig{}, fmt.Errorf("account not found: %s", selector)
}
