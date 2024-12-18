package config

import (
	"encoding/json"
	"os"
)

// Config holds all the configuration parameters, serving as a unified config.
type Config struct {
	DataStorageFilePath string           `json:"DataStorageFilePath,omitempty"`
	PoolCommissionRate  string           `json:"PoolCommissionRate"`
	Version             string           `json:"Version,omitempty"`
	PluginPath          string           `json:"PluginPath,omitempty"`
	PluginConfig        json.RawMessage  `json:"PluginConfig,omitempty"`
	PayoutLoopConfig    PayoutLoopConfig `json:"PayoutLoopConfig,omitempty"`
	APIConfig           APIConfig        `json:"APIConfig,omitempty"`
}

// LoadConfig reads a config file and unmarshals it into a unified Config struct.
func LoadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ParsePluginConfig unmarshals the PluginConfig field into a PluginConfig struct.
func (c *Config) ParsePluginConfig() (*PluginConfig, error) {
	if len(c.PluginConfig) == 0 {
		return nil, nil
	}

	var pluginCfg PluginConfig
	if err := json.Unmarshal(c.PluginConfig, &pluginCfg); err != nil {
		return nil, err
	}
	return &pluginCfg, nil
}

type APIConfig struct {
	ServerPort int    `json:"ServerPort,omitempty"`
	Region     string `json:"Region,omitempty"`
}
type PayoutLoopConfig struct {
	RPCUrl                   string `json:"RPCUrl,omitempty"`
	PrivateKeyStorePath      string `json:"PrivateKeyStorePath,omitempty"`
	PrivateKeyPassphrasePath string `json:"PrivateKeyPassphrasePath,omitempty"`
	Region                   string `json:"Region,omitempty"`
	PayoutFrequencySeconds   int    `json:"PayoutFrequencySeconds,omitempty"`
	PayoutThreshold          string `json:"PayoutThreshold,omitempty"`
}

// PluginConfig holds the plugin-specific settings for data loaders.
type PluginConfig struct {
	DBPath               string       `json:"db_path"`
	FetchIntervalSeconds int          `json:"fetch_interval_seconds"`
	Region               string       `json:"region"`
	DataSources          []DataSource `json:"data_sources"`
	PoolCommission       float64      `json:"pool_commission"`
}

// DataSource defines the settings for each data source.
type DataSource struct {
	Endpoint string `json:"endpoint"`
	NodeType string `json:"nodeType"`
}
