// Package config resolves runtime configuration from environment variables.
package config

type Config struct {
	Host            string
	Port            string
	DataDir         string
	ContentDir      string
	LabRepoDir      string
	OpenRouterKey   string
	OpenRouterModel string
}

// FromEnv reads configuration via getenv, applying defaults for unset values.
func FromEnv(getenv func(string) string) Config {
	get := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	return Config{
		// Loopback by default: the tour is a local app. Hosted deployments
		// (e.g. Cloud Run) set HOST=0.0.0.0 to accept external traffic.
		Host:            get("HOST", "127.0.0.1"),
		Port:            get("PORT", "8080"),
		DataDir:         get("DATA_DIR", "./data"),
		ContentDir:      get("CONTENT_DIR", "./content"),
		LabRepoDir:      getenv("LAB_REPO_DIR"),
		OpenRouterKey:   getenv("OPENROUTER_API_KEY"),
		OpenRouterModel: get("OPENROUTER_MODEL", "anthropic/claude-sonnet-4"),
	}
}
