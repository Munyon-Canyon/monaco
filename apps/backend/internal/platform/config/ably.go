package config

import "time"

type Ably struct {
	APIKey   string
	RESTHost string
}

func ablyFields() []field {
	return []field{
		text("ABLY_API_KEY", "", func(c *Config) *string { return &c.Ably.APIKey }).secret().
			requiredIn(EnvStaging, EnvProduction),
		text("ABLY_REST_HOST", "rest.ably.io", func(c *Config) *string { return &c.Ably.RESTHost }),
		duration("MONACO_TIMEOUT_ABLY", 2*time.Second, func(c *Config) *time.Duration { return &c.Timeouts.Ably }),
	}
}
