package config_test

import (
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/internal/config"
)

func TestFromEnvDefaults(t *testing.T) {
	cfg := config.FromEnv(func(string) string { return "" })
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", cfg.Host)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want ./data", cfg.DataDir)
	}
	if cfg.ContentDir != "./content" {
		t.Errorf("ContentDir = %q, want ./content", cfg.ContentDir)
	}
	if cfg.OpenRouterModel != "anthropic/claude-sonnet-4" {
		t.Errorf("OpenRouterModel = %q", cfg.OpenRouterModel)
	}
	if cfg.OpenRouterKey != "" || cfg.LabRepoDir != "" {
		t.Errorf("key/labrepo should default empty, got %q %q", cfg.OpenRouterKey, cfg.LabRepoDir)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	env := map[string]string{
		"PORT": "9999", "HOST": "0.0.0.0", "DATA_DIR": "/d", "CONTENT_DIR": "/c",
		"LAB_REPO_DIR": "/lab", "OPENROUTER_API_KEY": "sk", "OPENROUTER_MODEL": "x/y",
	}
	cfg := config.FromEnv(func(k string) string { return env[k] })
	if cfg.Port != "9999" || cfg.Host != "0.0.0.0" || cfg.DataDir != "/d" || cfg.ContentDir != "/c" ||
		cfg.LabRepoDir != "/lab" || cfg.OpenRouterKey != "sk" || cfg.OpenRouterModel != "x/y" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}
