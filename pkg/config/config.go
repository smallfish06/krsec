// Package config loads and validates krsec YAML configuration.
package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/smallfish06/krsec/pkg/broker"
)

// Listen address used when server.host or server.port is omitted.
const (
	DefaultHost = "localhost"
	DefaultPort = 8080
)

// Config is the root of config.yaml.
type Config struct {
	Server   ServerConfig    `yaml:"server"`
	Storage  StorageConfig   `yaml:"storage,omitempty"`
	KISProxy KISProxyConfig  `yaml:"kis_proxy,omitempty"`
	Accounts []AccountConfig `yaml:"accounts,omitempty"`
}

// ServerConfig is the HTTP listen address and the optional gateway API key.
//
// When APIKey is set, every request except /health and the Swagger UI must
// send it as "Authorization: Bearer <key>" or "X-API-Key: <key>". APIKeyEnv
// names an environment variable to read the key from instead.
type ServerConfig struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	APIKey    string `yaml:"api_key,omitempty"`
	APIKeyEnv string `yaml:"api_key_env,omitempty"`
}

// StorageConfig represents local persistence paths.
type StorageConfig struct {
	TokenDir        string `yaml:"token_dir"`
	OrderContextDir string `yaml:"order_context_dir"`
}

// KISProxyConfig represents KIS proxy behavior.
type KISProxyConfig struct {
	RateLimit KISProxyRateLimitConfig `yaml:"rate_limit,omitempty"`
}

// KISProxyRateLimitConfig limits outbound KIS upstream calls from the proxy.
type KISProxyRateLimitConfig struct {
	Enabled           *bool                             `yaml:"enabled,omitempty"`
	RequestsPerSecond float64                           `yaml:"requests_per_second,omitempty"`
	Burst             int                               `yaml:"burst,omitempty"`
	Overrides         []KISProxyRateLimitOverrideConfig `yaml:"overrides,omitempty"`
}

// KISProxyRateLimitOverrideConfig applies a stricter limit to selected KIS
// proxy endpoints.
type KISProxyRateLimitOverrideConfig struct {
	Path              string  `yaml:"path,omitempty"`
	TRID              string  `yaml:"tr_id,omitempty"`
	RequestsPerSecond float64 `yaml:"requests_per_second,omitempty"`
	Burst             int     `yaml:"burst,omitempty"`
}

// AccountConfig binds one broker account. AppKeyEnv and AppSecretEnv name
// environment variables that take precedence over the inline values.
type AccountConfig struct {
	Name         string `yaml:"name"`
	Broker       string `yaml:"broker"` // broker.CodeKIS, CodeKiwoom, CodeLS or CodeToss
	Sandbox      bool   `yaml:"sandbox"`
	AppKey       string `yaml:"app_key"`
	AppSecret    string `yaml:"app_secret"`
	AppKeyEnv    string `yaml:"app_key_env,omitempty"`
	AppSecretEnv string `yaml:"app_secret_env,omitempty"`
	AccountID    string `yaml:"account_id"`
	AccountSeq   string `yaml:"account_seq,omitempty"`
	MACAddress   string `yaml:"mac_address,omitempty"`
}

var accountIDPattern = regexp.MustCompile(`^\d{8}(-\d{2})?$`)
var kiwoomAccountIDPattern = regexp.MustCompile(`^\d{10}$`)

// Validate normalizes the config in place (trimming, defaults, environment
// lookups) and reports the first invalid value.
func (c *Config) Validate() error {
	c.Server.Host = strings.TrimSpace(c.Server.Host)
	if c.Server.Host == "" {
		c.Server.Host = DefaultHost
	}
	if c.Server.Port == 0 {
		c.Server.Port = DefaultPort
	}
	if c.Server.Port < 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	c.Server.APIKeyEnv = strings.TrimSpace(c.Server.APIKeyEnv)
	if c.Server.APIKeyEnv != "" {
		c.Server.APIKey = os.Getenv(c.Server.APIKeyEnv)
		if strings.TrimSpace(c.Server.APIKey) == "" {
			return fmt.Errorf("server.api_key_env %q is set but the environment variable is empty", c.Server.APIKeyEnv)
		}
	}
	c.Server.APIKey = strings.TrimSpace(c.Server.APIKey)

	c.Storage.TokenDir = strings.TrimSpace(c.Storage.TokenDir)
	c.Storage.OrderContextDir = strings.TrimSpace(c.Storage.OrderContextDir)
	if c.KISProxy.RateLimit.RequestsPerSecond < 0 {
		return fmt.Errorf("kis_proxy.rate_limit.requests_per_second must be greater than or equal to 0")
	}
	if c.KISProxy.RateLimit.Burst < 0 {
		return fmt.Errorf("kis_proxy.rate_limit.burst must be greater than or equal to 0")
	}
	for i := range c.KISProxy.RateLimit.Overrides {
		override := &c.KISProxy.RateLimit.Overrides[i]
		override.Path = strings.TrimSpace(override.Path)
		override.TRID = strings.TrimSpace(override.TRID)
		if override.Path == "" {
			return fmt.Errorf("kis_proxy.rate_limit.overrides[%d].path is required", i)
		}
		if override.RequestsPerSecond <= 0 {
			return fmt.Errorf("kis_proxy.rate_limit.overrides[%d].requests_per_second must be greater than 0", i)
		}
		if override.Burst < 0 {
			return fmt.Errorf("kis_proxy.rate_limit.overrides[%d].burst must be greater than or equal to 0", i)
		}
	}

	if len(c.Accounts) == 0 {
		return fmt.Errorf("at least one account is required")
	}

	seen := make(map[string]struct{}, len(c.Accounts))
	for i := range c.Accounts {
		acc := &c.Accounts[i]
		acc.Name = strings.TrimSpace(acc.Name)
		if acc.Name == "" {
			return fmt.Errorf("accounts[%d].name is required", i)
		}

		acc.Broker = strings.ToLower(strings.TrimSpace(acc.Broker))
		switch acc.Broker {
		case broker.CodeKIS:
		case broker.CodeKiwoom:
		case broker.CodeLS:
		case broker.CodeToss:
		default:
			return fmt.Errorf("accounts[%d].broker unsupported value %q (expected: %s|%s|%s|%s)", i, acc.Broker, broker.CodeKIS, broker.CodeKiwoom, broker.CodeLS, broker.CodeToss)
		}

		acc.AppKeyEnv = strings.TrimSpace(acc.AppKeyEnv)
		acc.AppSecretEnv = strings.TrimSpace(acc.AppSecretEnv)
		if acc.AppKeyEnv != "" {
			acc.AppKey = strings.TrimSpace(os.Getenv(acc.AppKeyEnv))
		}
		if acc.AppSecretEnv != "" {
			acc.AppSecret = strings.TrimSpace(os.Getenv(acc.AppSecretEnv))
		}

		acc.AppKey = strings.TrimSpace(acc.AppKey)
		if acc.AppKey == "" {
			return fmt.Errorf("accounts[%d].app_key is required%s", i, emptyEnvHint(acc.AppKeyEnv))
		}
		acc.AppSecret = strings.TrimSpace(acc.AppSecret)
		if acc.AppSecret == "" {
			return fmt.Errorf("accounts[%d].app_secret is required%s", i, emptyEnvHint(acc.AppSecretEnv))
		}

		accountID := strings.TrimSpace(acc.AccountID)
		switch acc.Broker {
		case broker.CodeKIS:
			if !accountIDPattern.MatchString(accountID) {
				return fmt.Errorf("accounts[%d].account_id invalid format %q for %s (expected: 12345678 or 12345678-01)", i, accountID, broker.CodeKIS)
			}
			if len(accountID) == 8 {
				accountID += "-01"
			}
		case broker.CodeKiwoom:
			if !kiwoomAccountIDPattern.MatchString(accountID) {
				return fmt.Errorf("accounts[%d].account_id invalid format %q for %s (expected: 10-digit number)", i, accountID, broker.CodeKiwoom)
			}
		case broker.CodeLS:
			if accountID == "" {
				return fmt.Errorf("accounts[%d].account_id is required for %s", i, broker.CodeLS)
			}
		case broker.CodeToss:
			if acc.Sandbox {
				return fmt.Errorf("accounts[%d].sandbox is not supported for %s", i, broker.CodeToss)
			}
			if accountID == "" {
				return fmt.Errorf("accounts[%d].account_id is required for %s", i, broker.CodeToss)
			}
			acc.AccountSeq = strings.TrimSpace(acc.AccountSeq)
			if acc.AccountSeq == "" {
				return fmt.Errorf("accounts[%d].account_seq is required for %s", i, broker.CodeToss)
			}
			if n, err := strconv.ParseInt(acc.AccountSeq, 10, 64); err != nil || n <= 0 {
				return fmt.Errorf("accounts[%d].account_seq invalid format %q for %s (expected: positive integer)", i, acc.AccountSeq, broker.CodeToss)
			}
		}
		acc.AccountID = accountID
		acc.MACAddress = strings.TrimSpace(acc.MACAddress)

		if _, ok := seen[accountID]; ok {
			return fmt.Errorf("duplicate account_id: %s", accountID)
		}
		seen[accountID] = struct{}{}
	}

	return nil
}

// emptyEnvHint explains a missing credential that was supposed to come from an
// environment variable.
func emptyEnvHint(envName string) string {
	if envName == "" {
		return ""
	}
	return fmt.Sprintf(" (environment variable %q is empty)", envName)
}

// Load reads and validates a YAML config file.
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename) //nolint:gosec // G304: the config path is caller-provided by design
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}
