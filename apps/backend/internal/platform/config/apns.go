package config

func (e *keysError) checkAPNs(c Config) {
	sends := c.Env == EnvStaging || c.Env == EnvProduction || c.APNs.KeyP8 != ""
	if sends {
		for _, k := range []struct{ name, value string }{
			{"APNS_KEY_P8", c.APNs.KeyP8},
			{"APNS_KEY_ID", c.APNs.KeyID},
			{"APNS_TEAM_ID", c.APNs.TeamID},
		} {
			if k.value == "" {
				e.missing = append(e.missing, k.name)
			}
		}
	}
	if c.Env == EnvProduction && c.APNs.BaseURL != "" {
		e.invalid = append(e.invalid, "APNS_BASE_URL (not allowed in production)")
	}
}
