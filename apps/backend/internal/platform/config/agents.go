package config

import "encoding/base64"

const agentKeyEncryptionKeyBytes = 32

type Agents struct {
	KeyEncryptionKey string
}

func agentsFields() []field {
	encryptionKey := field{
		key:  "AGENT_KEY_ENCRYPTION_KEY",
		want: "32 bytes, base64",
		set: func(c *Config, v string) bool {
			c.Agents.KeyEncryptionKey = v
			raw, err := base64.StdEncoding.DecodeString(v)
			return v == "" || err == nil && len(raw) == agentKeyEncryptionKeyBytes
		},
		get: func(c *Config) string { return c.Agents.KeyEncryptionKey },
	}
	return []field{encryptionKey.secret().requiredIn(EnvStaging, EnvProduction)}
}
