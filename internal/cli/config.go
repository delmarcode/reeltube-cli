package cli

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/the-del-mar-code-company/reeltube-cli/internal/client"
	"gopkg.in/yaml.v3"
)

// Config holds CLI configuration
type Config struct {
	APIKey  string
	BaseURL string
	Debug   bool
	Logger  *slog.Logger
	Client  *client.Client
}

// FileConfig represents the YAML configuration file structure
type FileConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

// LoadConfigFile loads configuration from ~/.config/reeltube/config.yaml
func LoadConfigFile() (*FileConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(homeDir, ".config", "reeltube", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Config file doesn't exist, not an error
		}
		return nil, err
	}

	var cfg FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}
