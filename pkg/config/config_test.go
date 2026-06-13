package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadValidConfigNormalizesValues(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "127.0.0.1"
  port: 9090
accounts:
  - name: "main"
    broker: KIS
    sandbox: true
    app_key: "k"
    app_secret: "s"
    account_id: "12345678"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if len(cfg.Accounts) != 1 {
		t.Fatalf("accounts length = %d, want 1", len(cfg.Accounts))
	}
	if cfg.Accounts[0].Broker != "kis" {
		t.Fatalf("broker = %q, want %q", cfg.Accounts[0].Broker, "kis")
	}
	if cfg.Accounts[0].AccountID != "12345678-01" {
		t.Fatalf("account_id = %q, want %q", cfg.Accounts[0].AccountID, "12345678-01")
	}
}

func TestLoadValidLSConfig(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "127.0.0.1"
  port: 9090
accounts:
  - name: "ls-main"
    broker: LS
    sandbox: false
    app_key: "k"
    app_secret: "s"
    account_id: "  ls-main  "
    mac_address: "  00:11:22:33:44:55  "
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if len(cfg.Accounts) != 1 {
		t.Fatalf("accounts length = %d, want 1", len(cfg.Accounts))
	}
	acc := cfg.Accounts[0]
	if acc.Broker != "ls" {
		t.Fatalf("broker = %q, want ls", acc.Broker)
	}
	if acc.AccountID != "ls-main" {
		t.Fatalf("account_id = %q, want ls-main", acc.AccountID)
	}
	if acc.MACAddress != "00:11:22:33:44:55" {
		t.Fatalf("mac_address = %q", acc.MACAddress)
	}
}

func TestLoadValidTossConfigResolvesEnvCredentials(t *testing.T) {
	t.Setenv("TOSS_TEST_CLIENT_ID", "client-id")
	t.Setenv("TOSS_TEST_CLIENT_SECRET", "client-secret")
	path := writeTempConfig(t, `
server:
  host: "127.0.0.1"
  port: 9090
accounts:
  - name: "toss-main"
    broker: TOSS
    sandbox: false
    app_key_env: "TOSS_TEST_CLIENT_ID"
    app_secret_env: "TOSS_TEST_CLIENT_SECRET"
    account_id: "  toss-main  "
    account_seq: "  1  "
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if len(cfg.Accounts) != 1 {
		t.Fatalf("accounts length = %d, want 1", len(cfg.Accounts))
	}
	acc := cfg.Accounts[0]
	if acc.Broker != "toss" {
		t.Fatalf("broker = %q, want toss", acc.Broker)
	}
	if acc.AppKey != "client-id" || acc.AppSecret != "client-secret" {
		t.Fatalf("credentials not resolved from env: %+v", acc)
	}
	if acc.AccountID != "toss-main" || acc.AccountSeq != "1" {
		t.Fatalf("unexpected account fields: %+v", acc)
	}
}

func TestLoadRejectsTossSandbox(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "127.0.0.1"
  port: 9090
accounts:
  - name: "toss-main"
    broker: toss
    sandbox: true
    app_key: "k"
    app_secret: "s"
    account_id: "toss-main"
    account_seq: "1"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "sandbox is not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidTossAccountSeq(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "127.0.0.1"
  port: 9090
accounts:
  - name: "toss-main"
    broker: toss
    sandbox: false
    app_key: "k"
    app_secret: "s"
    account_id: "toss-main"
    account_seq: "abc"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "account_seq invalid format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadStorageConfig(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "127.0.0.1"
  port: 9090
storage:
  token_dir: "  .cache/tokens  "
  order_context_dir: "  .cache/orders  "
accounts:
  - name: "main"
    broker: kis
    sandbox: true
    app_key: "k"
    app_secret: "s"
    account_id: "12345678-01"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Storage.TokenDir != ".cache/tokens" {
		t.Fatalf("token_dir = %q, want %q", cfg.Storage.TokenDir, ".cache/tokens")
	}
	if cfg.Storage.OrderContextDir != ".cache/orders" {
		t.Fatalf("order_context_dir = %q, want %q", cfg.Storage.OrderContextDir, ".cache/orders")
	}
}

func TestLoadKISProxyRateLimitConfig(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "127.0.0.1"
  port: 9090
kis_proxy:
  rate_limit:
    enabled: true
    requests_per_second: 12.5
    burst: 1
    overrides:
      - path: "/uapi/overseas-price/v1/quotations/inquire-time-indexchartprice"
        tr_id: "FHKST03030200"
        requests_per_second: 5
        burst: 1
accounts:
  - name: "main"
    broker: kis
    sandbox: true
    app_key: "k"
    app_secret: "s"
    account_id: "12345678-01"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.KISProxy.RateLimit.Enabled == nil || !*cfg.KISProxy.RateLimit.Enabled {
		t.Fatalf("rate_limit.enabled = %v, want true", cfg.KISProxy.RateLimit.Enabled)
	}
	if cfg.KISProxy.RateLimit.RequestsPerSecond != 12.5 {
		t.Fatalf("requests_per_second = %v, want 12.5", cfg.KISProxy.RateLimit.RequestsPerSecond)
	}
	if cfg.KISProxy.RateLimit.Burst != 1 {
		t.Fatalf("burst = %d, want 1", cfg.KISProxy.RateLimit.Burst)
	}
	if len(cfg.KISProxy.RateLimit.Overrides) != 1 {
		t.Fatalf("overrides len = %d, want 1", len(cfg.KISProxy.RateLimit.Overrides))
	}
	override := cfg.KISProxy.RateLimit.Overrides[0]
	if override.Path != "/uapi/overseas-price/v1/quotations/inquire-time-indexchartprice" {
		t.Fatalf("override.path = %q", override.Path)
	}
	if override.TRID != "FHKST03030200" {
		t.Fatalf("override.tr_id = %q", override.TRID)
	}
	if override.RequestsPerSecond != 5 {
		t.Fatalf("override.requests_per_second = %v, want 5", override.RequestsPerSecond)
	}
	if override.Burst != 1 {
		t.Fatalf("override.burst = %d, want 1", override.Burst)
	}
}

func TestLoadRejectsInvalidKISProxyRateLimit(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "127.0.0.1"
  port: 9090
kis_proxy:
  rate_limit:
    requests_per_second: -1
accounts:
  - name: "main"
    broker: kis
    sandbox: true
    app_key: "k"
    app_secret: "s"
    account_id: "12345678-01"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "kis_proxy.rate_limit.requests_per_second") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidKISProxyRateLimitOverride(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "127.0.0.1"
  port: 9090
kis_proxy:
  rate_limit:
    overrides:
      - path: "/uapi/overseas-price/v1/quotations/inquire-time-indexchartprice"
        requests_per_second: 0
accounts:
  - name: "main"
    broker: kis
    sandbox: true
    app_key: "k"
    app_secret: "s"
    account_id: "12345678-01"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "kis_proxy.rate_limit.overrides[0].requests_per_second") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsLegacyKISConfig(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "0.0.0.0"
  port: 8080
kis:
  sandbox: false
  app_key: "k"
  app_secret: "s"
  account_id: "87654321-01"
`)

	_, err := Load(path)
	if err != nil {
		if !strings.Contains(err.Error(), "field kis not found in type config.Config") {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	t.Fatal("Load() expected error, got nil")
}

func TestLoadRejectsUnsupportedBroker(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "0.0.0.0"
  port: 8080
accounts:
  - name: "main"
    broker: future
    sandbox: false
    app_key: "k"
    app_secret: "s"
    account_id: "12345678-01"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "broker unsupported value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidAccountID(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "0.0.0.0"
  port: 8080
accounts:
  - name: "main"
    broker: kis
    sandbox: false
    app_key: "k"
    app_secret: "s"
    account_id: "12-01"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "account_id invalid format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidKiwoomAccountID(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "0.0.0.0"
  port: 8080
accounts:
  - name: "main"
    broker: kiwoom
    sandbox: false
    app_key: "k"
    app_secret: "s"
    account_id: "12345678-01"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "for kiwoom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "0.0.0.0"
  port: 8080
accounts:
  - name: "main"
    broker: kis
    sandbox: false
    app_key: "k"
    app_secret: "s"
    account_id: "12345678-01"
    not_allowed: true
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found in type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsDuplicateAccountID(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "0.0.0.0"
  port: 8080
accounts:
  - name: "main"
    broker: kis
    sandbox: false
    app_key: "k1"
    app_secret: "s1"
    account_id: "12345678"
  - name: "sub"
    broker: kis
    sandbox: false
    app_key: "k2"
    app_secret: "s2"
    account_id: "12345678-01"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate account_id") {
		t.Fatalf("unexpected error: %v", err)
	}
}
