package config

type Listings struct {
	SyncEnabled      bool
	OndoBaseURL      string
	CoinbaseBaseURL  string
	RobinhoodBaseURL string
}

func listingsFields() []field {
	return []field{
		deployedByDefault("LISTINGS_SYNC_ENABLED", func(c *Config) *bool { return &c.Listings.SyncEnabled }),
		text("ONDO_BASE_URL", "https://app.ondo.finance", func(c *Config) *string { return &c.Listings.OndoBaseURL }),
		text("COINBASE_BASE_URL", "https://api.coinbase.com",
			func(c *Config) *string { return &c.Listings.CoinbaseBaseURL }),
		text("ROBINHOOD_BASE_URL", "https://api.robinhood.com",
			func(c *Config) *string { return &c.Listings.RobinhoodBaseURL }),
	}
}
