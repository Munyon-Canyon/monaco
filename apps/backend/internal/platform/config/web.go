package config

import "net/url"

type Web struct {
	FundPageURL string
}

const (
	DeployedFundPageURL = "https://monacolabs.xyz/fund"
	LocalFundPageURL    = "http://localhost:5173/fund"
)

func (c Config) FundPageURL() string {
	switch {
	case c.Web.FundPageURL != "":
		return c.Web.FundPageURL
	case c.Env.Deployed():
		return DeployedFundPageURL
	default:
		return LocalFundPageURL
	}
}

func webFields() []field {
	return []field{
		pageURL("FUND_PAGE_URL", func(c *Config) *string { return &c.Web.FundPageURL }),
	}
}

func pageURL(key string, at func(*Config) *string) field {
	f := text(key, "", at)
	f.want = "empty or an absolute http(s) URL with no query or fragment"
	f.set = func(c *Config, v string) bool {
		*at(c) = v
		if v == "" {
			return true
		}
		u, err := url.Parse(v)
		return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" &&
			u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
	}
	return f
}
