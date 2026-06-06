package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/smallfish06/krsec/pkg/broker"
)

// Config represents the application configuration
type Config struct {
	Server   ServerConfig    `yaml:"server"`
	Storage  StorageConfig   `yaml:"storage,omitempty"`
	KISProxy KISProxyConfig  `yaml:"kis_proxy,omitempty"`
	Accounts []AccountConfig `yaml:"accounts,omitempty"`
}

// ServerConfig represents server configuration
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
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
	Enabled           *bool   `yaml:"enabled,omitempty"`
	RequestsPerSecond float64 `yaml:"requests_per_second,omitempty"`
	Burst             int     `yaml:"burst,omitempty"`
}

// AccountConfig represents a broker account configuration
type AccountConfig struct {
	Name       string `yaml:"name"`
	Broker     string `yaml:"broker"` // currently supported: broker.CodeKIS, broker.CodeKiwoom, broker.CodeLS
	Sandbox    bool   `yaml:"sandbox"`
	AppKey     string `yaml:"app_key"`
	AppSecret  string `yaml:"app_secret"`
	AccountID  string `yaml:"account_id"`
	MACAddress string `yaml:"mac_address,omitempty"`
}

var accountIDPattern = regexp.MustCompile(`^\d{8}(-\d{2})?$`)
var kiwoomAccountIDPattern = regexp.MustCompile(`^\d{10}$`)

// Validate validates and normalizes configuration values.
func (c *Config) Validate() error {
	c.Server.Host = strings.TrimSpace(c.Server.Host)
	if c.Server.Host == "" {
		return fmt.Errorf("server.host is required")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}

	c.Storage.TokenDir = strings.TrimSpace(c.Storage.TokenDir)
	c.Storage.OrderContextDir = strings.TrimSpace(c.Storage.OrderContextDir)
	if c.KISProxy.RateLimit.RequestsPerSecond < 0 {
		return fmt.Errorf("kis_proxy.rate_limit.requests_per_second must be greater than or equal to 0")
	}
	if c.KISProxy.RateLimit.Burst < 0 {
		return fmt.Errorf("kis_proxy.rate_limit.burst must be greater than or equal to 0")
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
		default:
			return fmt.Errorf("accounts[%d].broker unsupported value %q (expected: %s|%s|%s)", i, acc.Broker, broker.CodeKIS, broker.CodeKiwoom, broker.CodeLS)
		}

		acc.AppKey = strings.TrimSpace(acc.AppKey)
		if acc.AppKey == "" {
			return fmt.Errorf("accounts[%d].app_key is required", i)
		}
		acc.AppSecret = strings.TrimSpace(acc.AppSecret)
		if acc.AppSecret == "" {
			return fmt.Errorf("accounts[%d].app_secret is required", i)
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

// Load loads configuration from a YAML file
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
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
