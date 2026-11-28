package server

import (
	"context"
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

const (
	// startupAuthTimeout covers one token-issuance throttle window (60s) plus
	// the HTTP round trip.
	startupAuthTimeout = 2 * time.Minute

	kisSymbolBootstrapTimeout = 45 * time.Second
	kisSymbolReloadTimeout    = 90 * time.Second
	// KIS republishes its symbol master files daily.
	kisSymbolReloadInterval = 24 * time.Hour
)

// init binds a built-in broker adapter to every configured account and starts
// authenticating them in the background, so the server can accept requests
// before the brokers have answered.
func (s *Server) init(cfg *config.Config) *Server {
	kisTokenManager := kis.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	kiwoomTokenManager := kiwoom.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	lsTokenManager := ls.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)
	tossTokenManager := toss.NewFileTokenManagerWithDir(cfg.Storage.TokenDir)

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
			go s.keepKISSymbolsFresh(account.Name, adapter)
			brk = adapter
		case broker.CodeKiwoom:
			brk = kiwoomadapter.NewAdapterWithOptions(
				account.Sandbox,
				account.AccountID,
				kiwoomTokenManager,
				cfg.Storage.OrderContextDir,
				s.logger,
			)
		case broker.CodeLS:
			brk = lsadapter.NewAdapterWithOptions(
				account.Sandbox,
				account.AccountID,
				lsTokenManager,
				account.MACAddress,
				s.logger,
			)
		case broker.CodeToss:
			brk = tossadapter.NewAdapterWithOptions(
				account.Sandbox,
				account.AccountID,
				account.AccountSeq,
				tossTokenManager,
				s.logger,
			)
		default:
			s.logger.Warn("unknown broker type", "broker", account.Broker)
			continue
		}

		go s.authenticateAtStartup(account.Name, brk, broker.Credentials{
			AppKey:    account.AppKey,
			AppSecret: account.AppSecret,
		})
		s.brokers[account.AccountID] = brk
	}

	return s
}

func (s *Server) authenticateAtStartup(name string, brk broker.Broker, creds broker.Credentials) {
	ctx, cancel := context.WithTimeout(s.lifecycle, startupAuthTimeout)
	defer cancel()

	if _, err := brk.Authenticate(ctx, creds); err != nil {
		s.logger.Warn("failed to authenticate account", "account", name, "error", err)
		return
	}
	s.logger.Info("authenticated account", "account", name)
}

// keepKISSymbolsFresh loads the KIS symbol master files and reloads them
// daily until the server closes.
func (s *Server) keepKISSymbolsFresh(name string, adapter *kisadapter.Adapter) {
	s.loadKISSymbols(name, "bootstrap", kisSymbolBootstrapTimeout, adapter.BootstrapSymbols)

	ticker := time.NewTicker(kisSymbolReloadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.lifecycle.Done():
			return
		case <-ticker.C:
			s.loadKISSymbols(name, "reload", kisSymbolReloadTimeout, adapter.ReloadSymbols)
		}
	}
}

func (s *Server) loadKISSymbols(name, phase string, timeout time.Duration, load func(context.Context) (int, error)) {
	ctx, cancel := context.WithTimeout(s.lifecycle, timeout)
	defer cancel()

	count, err := load(ctx)
	if err != nil {
		s.logger.Warn("symbol "+phase+" failed", "account", name, "error", err)
		return
	}
	s.logger.Info("symbol "+phase+" done", "account", name, "count", count)
}
