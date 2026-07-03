package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Cloudflare CloudflareConfig `mapstructure:"cloudflare"`
	NIM        NIMConfig        `mapstructure:"nim"`
	Sonarr     ServiceConfig    `mapstructure:"sonarr"`
	Radarr     ServiceConfig    `mapstructure:"radarr"`
	SABnzbd    ServiceConfig    `mapstructure:"sabnzbd"`
	Plex       PlexConfig       `mapstructure:"plex"`
	DB         DBConfig         `mapstructure:"db"`
}

type ServerConfig struct {
	Port      int    `mapstructure:"port"`
	LANOnlyKey string `mapstructure:"lan_only_key"`
}

type CloudflareConfig struct {
	AccessTeamDomain string `mapstructure:"access_team_domain"`
}

type NIMConfig struct {
	BaseURL      string `mapstructure:"base_url"`
	APIKey       string `mapstructure:"api_key"`
	DefaultModel string `mapstructure:"default_model"`
	CFClientID   string `mapstructure:"cf_client_id"`
	CFSecret     string `mapstructure:"cf_secret"`
}

type ServiceConfig struct {
	URL    string `mapstructure:"url"`
	APIKey string `mapstructure:"api_key"`
}

type PlexConfig struct {
	URL   string `mapstructure:"url"`
	Token string `mapstructure:"token"`
}

type DBConfig struct {
	Path string `mapstructure:"path"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/superol1n")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("config: read: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}
	return &cfg, nil
}
