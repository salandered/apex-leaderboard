package apexredis

import "os"

const defaultURL = "redis://localhost:6379/0"

// Config is a Redis connection setup. Its zero value resolves to the documented default.
type Config struct {
	URL string
}

// Fills the empty fields with their defaults.
// [New] must call it.
// Idempotent.
func (c *Config) resolve() {
	if c.URL == "" {
		c.URL = defaultURL
	}
}

// Env vars:
//   - REDIS_URL: connection url (default [defaultURL])
//
// TODO: The error is what REDIS_PASSWORD and url validation will report (redis security stream).
// No error currently.
func ConfigFromEnv() (Config, error) {
	cfg := Config{URL: os.Getenv("REDIS_URL")}
	cfg.resolve()
	return cfg, nil
}
