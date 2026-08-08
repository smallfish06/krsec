package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/smallfish06/krsec/internal/kis"
	kisadapter "github.com/smallfish06/krsec/internal/kis/adapter"
	"github.com/smallfish06/krsec/internal/kiwoom"
	kiwoomadapter "github.com/smallfish06/krsec/internal/kiwoom/adapter"
	"github.com/smallfish06/krsec/internal/ls"
	lsadapter "github.com/smallfish06/krsec/internal/ls/adapter"
	"github.com/smallfish06/krsec/internal/toss"
	tossadapter "github.com/smallfish06/krsec/internal/toss/adapter"
	"github.com/smallfish06/krsec/pkg/broker"
	"github.com/smallfish06/krsec/pkg/config"
)

func (s *Server) init(cfg *config.Config) *Server {

	kisTokenManager := kis.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	kiwoomTokenManager := kiwoom.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	lsTokenManager := ls.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	tossTokenManager := toss.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)

	// Initialize brokers for each account
	for _, account := range cfg.Accounts {
		var brk broker.Broker
		switch account.Broker {
		case broker.CodeKIS:
			adapter := kisadapter.NewAdapterWithOptions(
				account.Sandbox,
				account.AccountID,
				kisTokenManager,
				cfg.Storage.OrderContextDir,
				s.logger,
			)
			creds := broker.Credentials{
				AppKey:    account.AppKey,
				AppSecret: account.AppSecret,
			}
			s.authenticateInBackground(account.Name, adapter, creds)

			// Bootstrap symbol master files in background.
			go func(name string, a *kisadapter.Adapter, logger *slog.Logger) {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				count, err := a.BootstrapSymbols(ctx)
				if err != nil {
					logger.Warn("symbol bootstrap failed", "account", name, "error", err)
				} else {
					logger.Info("bootstrapped symbol records", "account", name, "count", count)
				}

				// Keep symbol master cache fresh (KIS master files change over time).
				ticker := time.NewTicker(24 * time.Hour)
				defer ticker.Stop()
				for range ticker.C {
					reloadCtx, reloadCancel := context.WithTimeout(context.Background(), 90*time.Second)
					count, err := a.ReloadSymbols(reloadCtx)
					reloadCancel()
					if err != nil {
						logger.Warn("symbol reload failed", "account", name, "error", err)
						continue
					}
					logger.Info("reloaded symbol records", "account", name, "count", count)
				}
			}(account.Name, adapter, s.logger)
			brk = adapter
		case broker.CodeKiwoom:
			adapter := kiwoomadapter.NewAdapterWithOptions(
				account.Sandbox,
				account.AccountID,
				kiwoomTokenManager,
				cfg.Storage.OrderContextDir,
				s.logger,
			)
			creds := broker.Credentials{
				AppKey:    account.AppKey,
				AppSecret: account.AppSecret,
			}
			s.authenticateInBackground(account.Name, adapter, creds)
			brk = adapter
		case broker.CodeLS:
			adapter := lsadapter.NewAdapterWithOptions(
				account.Sandbox,
				account.AccountID,
				lsTokenManager,
				account.MACAddress,
				s.logger,
			)
			creds := broker.Credentials{
				AppKey:    account.AppKey,
				AppSecret: account.AppSecret,
			}
			s.authenticateInBackground(account.Name, adapter, creds)
			brk = adapter
		case broker.CodeToss:
			adapter := tossadapter.NewAdapterWithOptions(
				account.Sandbox,
				account.AccountID,
				account.AccountSeq,
				tossTokenManager,
				s.logger,
			)
			creds := broker.Credentials{
				AppKey:    account.AppKey,
				AppSecret: account.AppSecret,
			}
			s.authenticateInBackground(account.Name, adapter, creds)
			brk = adapter
		default:
			s.logger.Warn("unknown broker type", "broker", account.Broker)
			continue
		}
		s.brokers[account.AccountID] = brk
	}

	return s
}

func (s *Server) authenticateInBackground(name string, brk broker.Broker, creds broker.Credentials) {
	go func() {
		if _, err := brk.Authenticate(context.Background(), creds); err != nil {
			s.logger.Warn("failed to authenticate account", "account", name, "error", err)
			return
		}
		s.logger.Info("authenticated account", "account", name)
	}()
}
