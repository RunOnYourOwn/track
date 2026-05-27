package ado

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Org    string       `json:"org"`
	PatEnv string       `json:"pat_env"`
	Email  string       `json:"email"`
	Syncs  []SyncConfig `json:"syncs"`
}

type SyncConfig struct {
	Project      string `json:"project"`
	Team         string `json:"team"`
	TrackProject string `json:"track_project"`
	AssignedTo   string `json:"assigned_to,omitempty"` // filter by email; "me" uses config email
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".track", "ado.json")
}

func LoadConfig() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no ADO config found at %s — run 'track ado config' to set up", path)
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.PatEnv == "" {
		cfg.PatEnv = "TRACK_ADO_PAT"
	}

	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (c *Config) PAT() (string, error) {
	pat := os.Getenv(c.PatEnv)
	if pat == "" {
		return "", fmt.Errorf("env var %s is not set", c.PatEnv)
	}
	return pat, nil
}
