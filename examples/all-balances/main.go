package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	pkgadapter "github.com/smallfish06/krsec/pkg/adapter"
	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
	"github.com/smallfish06/krsec/pkg/kis"
	"github.com/smallfish06/krsec/pkg/kiwoom"
	tokencache "github.com/smallfish06/krsec/pkg/token"
)

type accountResult struct {
	AccountID   string            `json:"account_id"`
	AccountName string            `json:"account_name"`
	Broker      string            `json:"broker"`
	Sandbox     bool              `json:"sandbox"`
	Balance     *broker.Balance   `json:"balance,omitempty"`
	Positions   []broker.Position `json:"positions,omitempty"`
	Error       string            `json:"error,omitempty"`
	DurationMS  int64             `json:"duration_ms"`
}

type output struct {
	GeneratedAt string          `json:"generated_at"`
	Summary     summary         `json:"summary"`
	Accounts    []accountResult `json:"accounts"`
}

type summary struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	withPositions := flag.Bool("positions", true, "Include positions in output")
	timeout := flag.Duration("timeout", 20*time.Second, "Per-account request timeout")
	brokerFilter := flag.String("broker", "", "Optional broker filter (kis|kiwoom)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	filter := strings.ToLower(strings.TrimSpace(*brokerFilter))
	if filter != "" && filter != "kis" && filter != "kiwoom" {
		slog.Error("invalid broker filter", "filter", *brokerFilter, "expected", "kis|kiwoom")
		os.Exit(1)
	}

	kisTokenManager := kis.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	kiwoomTokenManager := kiwoom.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)

	results := make([]accountResult, 0, len(cfg.Accounts))
	for _, acc := range cfg.Accounts {
		if filter != "" && strings.ToLower(strings.TrimSpace(acc.Broker)) != filter {
			continue
		}

		start := time.Now()
		item := accountResult{
			AccountID:   acc.AccountID,
			AccountName: acc.Name,
			Broker:      acc.Broker,
			Sandbox:     acc.Sandbox,
		}

		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		brk, err := buildBroker(acc, cfg, kisTokenManager, kiwoomTokenManager)
		if err != nil {
			cancel()
			item.Error = err.Error()
			item.DurationMS = time.Since(start).Milliseconds()
			results = append(results, item)
			continue
		}

		_, err = brk.Authenticate(ctx, broker.Credentials{AppKey: acc.AppKey, AppSecret: acc.AppSecret})
		if err != nil {
			cancel()
			item.Error = "authenticate: " + err.Error()
			item.DurationMS = time.Since(start).Milliseconds()
			results = append(results, item)
			continue
		}

		bal, err := brk.GetBalance(ctx, acc.AccountID)
		if err != nil {
			cancel()
			item.Error = "get balance: " + err.Error()
			item.DurationMS = time.Since(start).Milliseconds()
			results = append(results, item)
			continue
		}
		item.Balance = bal

		if *withPositions {
			positions, err := brk.GetPositions(ctx, acc.AccountID)
			if err != nil {
				cancel()
				item.Error = "get positions: " + err.Error()
				item.DurationMS = time.Since(start).Milliseconds()
				results = append(results, item)
				continue
			}
			item.Positions = positions
		}

		cancel()
		item.DurationMS = time.Since(start).Milliseconds()
		results = append(results, item)
	}

	out := output{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Accounts:    results,
		Summary: summary{
			Total: len(results),
		},
	}
	for _, item := range results {
		if strings.TrimSpace(item.Error) == "" {
			out.Summary.Success++
		} else {
			out.Summary.Failed++
		}
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		slog.Error("marshal output", "error", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func buildBroker(
	acc config.AccountConfig,
	cfg *config.Config,
	kisTokenManager tokencache.Manager,
	kiwoomTokenManager tokencache.Manager,
) (broker.Broker, error) {
	switch strings.ToLower(strings.TrimSpace(acc.Broker)) {
	case "kis":
		return kis.NewAdapterWithOptions(acc.Sandbox, acc.AccountID, pkgadapter.Options{
			TokenManager:    kisTokenManager,
			OrderContextDir: cfg.Storage.OrderContextDir,
		}), nil
	case "kiwoom":
		return kiwoom.NewAdapterWithOptions(acc.Sandbox, acc.AccountID, pkgadapter.Options{
			TokenManager:    kiwoomTokenManager,
			OrderContextDir: cfg.Storage.OrderContextDir,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported broker: %s", acc.Broker)
	}
}
