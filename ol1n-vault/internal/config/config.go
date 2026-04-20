package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Vault       VaultConfig
	NNTP        NNTPConfig
	NOWPayments NOWPaymentsConfig
	Polygon     PolygonConfig
}

type VaultConfig struct {
	Port          int
	PublicBaseURL string
	DBPath        string
	NZBDir        string
	FilesDir      string
	TmpDir        string
	FilesTTL      time.Duration
	APIKey        string
}

type NNTPConfig struct {
	Host        string
	Port        int
	TLS         bool
	User        string
	Pass        string
	Connections int
	Newsgroup   string
}

type NOWPaymentsConfig struct {
	APIKey     string
	IPNSecret  string
	APIBase    string
	SuccessURL string
	CancelURL  string
	PriceUSD   float64
}

type PolygonConfig struct {
	RPCURL     string
	PrivateKey string
}

func Load() (*Config, error) {
	cfg := &Config{
		Vault: VaultConfig{
			Port:          envInt("VAULT_PORT", 8090),
			PublicBaseURL: envStr("VAULT_PUBLIC_BASE_URL", "https://super.ol1n.com"),
			DBPath:        envStr("VAULT_DB_PATH", "/data/vault/vault.db"),
			NZBDir:        envStr("VAULT_NZB_DIR", "/data/vault/nzb"),
			FilesDir:      envStr("VAULT_FILES_DIR", "/data/vault/files"),
			TmpDir:        envStr("VAULT_TMP_DIR", "/data/vault/tmp"),
			FilesTTL:      time.Duration(envInt("VAULT_FILES_TTL_HOURS", 24)) * time.Hour,
			APIKey:        envStr("VAULT_API_KEY", ""),
		},
		NNTP: NNTPConfig{
			Host:        envStr("NNTP_HOST", ""),
			Port:        envInt("NNTP_PORT", 563),
			TLS:         envBool("NNTP_TLS", true),
			User:        envStr("NNTP_USER", ""),
			Pass:        envStr("NNTP_PASS", ""),
			Connections: envInt("NNTP_CONNECTIONS", 8),
			Newsgroup:   envStr("NNTP_NEWSGROUP", "alt.binaries.test"),
		},
		NOWPayments: NOWPaymentsConfig{
			APIKey:     envStr("NOWPAYMENTS_API_KEY", ""),
			IPNSecret:  envStr("NOWPAYMENTS_IPN_SECRET", ""),
			APIBase:    envStr("NOWPAYMENTS_API_BASE", "https://api.nowpayments.io/v1"),
			SuccessURL: envStr("NOWPAYMENTS_SUCCESS_URL", ""),
			CancelURL:  envStr("NOWPAYMENTS_CANCEL_URL", ""),
			PriceUSD:   envFloat("UPLOAD_PRICE_USD", 0.50),
		},
		Polygon: PolygonConfig{
			RPCURL:     envStr("POLYGON_RPC_URL", ""),
			PrivateKey: envStr("POLYGON_PRIVATE_KEY", ""),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	var missing []string
	if c.NNTP.Host == "" {
		missing = append(missing, "NNTP_HOST")
	}
	if c.NNTP.User == "" {
		missing = append(missing, "NNTP_USER")
	}
	if c.NNTP.Pass == "" {
		missing = append(missing, "NNTP_PASS")
	}
	if c.NOWPayments.APIKey == "" {
		missing = append(missing, "NOWPAYMENTS_API_KEY")
	}
	if c.NOWPayments.IPNSecret == "" {
		missing = append(missing, "NOWPAYMENTS_IPN_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required env vars: %s", strings.Join(missing, ", "))
	}
	return nil
}

func envStr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return fallback
}
