package config

import (
	"net/url"
	"strings"
)

type Web struct {
	FundPageURL    string
	AllowedOrigins string
}

const (
	DeployedFundPageURL = "https://monacolabs.xyz/fund"
	LocalFundPageURL    = "http://localhost:5173/fund"
	DeployedWebOrigin   = "https://monacolabs.xyz"
	LocalWebOrigin      = "http://localhost:5173"
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

func (c Config) WebAllowedOrigins() []string {
	switch {
	case c.Web.AllowedOrigins != "":
		return strings.Split(c.Web.AllowedOrigins, ",")
	case c.Env.Deployed():
		return []string{DeployedWebOrigin}
	default:
		return []string{DeployedWebOrigin, LocalWebOrigin}
	}
}

func webFields() []field {
	return []field{
		pageURL("FUND_PAGE_URL", func(c *Config) *string { return &c.Web.FundPageURL }),
		origins("WEB_ALLOWED_ORIGINS", func(c *Config) *string { return &c.Web.AllowedOrigins }),
	}
}

func origins(key string, at func(*Config) *string) field {
	f := text(key, "", at)
	f.want = "empty or comma-separated http(s) origins with no path, such as https://monacolabs.xyz"
	f.set = func(c *Config, v string) bool {
		*at(c) = v
		if v == "" {
			return true
		}
		for _, origin := range strings.Split(v, ",") {
			u, err := url.Parse(origin)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" ||
				u.Scheme+"://"+u.Host != origin {
				return false
			}
		}
		return true
	}
	return f
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
