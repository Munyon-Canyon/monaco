package config_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestFundPageURL_isTheSetURLElseTheDeployedPageElseTheLocalOne(t *testing.T) {
	t.Parallel()
	tests := []struct {
		env, set, want string
	}{
		{"local", "", config.LocalFundPageURL},
		{"test", "", config.LocalFundPageURL},
		{"staging", "", config.DeployedFundPageURL},
		{"production", "", config.DeployedFundPageURL},
		{"production", "https://fund.example/fund", "https://fund.example/fund"},
		{"local", "http://127.0.0.1:5173/fund", "http://127.0.0.1:5173/fund"},
	}
	for _, tt := range tests {
		cfg := config.Config{Env: config.Env(tt.env), Web: config.Web{FundPageURL: tt.set}}
		if got := cfg.FundPageURL(); got != tt.want {
			t.Errorf("env %s with %q: FundPageURL() = %q, want %q", tt.env, tt.set, got, tt.want)
		}
	}
}

func TestLoad_refusesAFundPageURLThatCannotTakeTheTokenQuery(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{
		"monacolabs.xyz/fund", "ftp://monacolabs.xyz/fund", "https:///fund", "https://monacolabs.xyz/fund?x=1",
		"https://monacolabs.xyz/fund#top", "https://monacolabs.xyz/fund?", "://bad",
	} {
		_, err := config.Load(append(required(), "FUND_PAGE_URL="+bad))
		want := "config.Load: invalid_input: invalid FUND_PAGE_URL " +
			"(empty or an absolute http(s) URL with no query or fragment)"
		if err == nil || err.Error() != want {
			t.Errorf("FUND_PAGE_URL=%s: Load error = %v, want %q", bad, err, want)
		}
	}
}

func TestWebAllowedOrigins_isTheSetListElseTheSiteAddingLocalhostOutsideDeploys(t *testing.T) {
	t.Parallel()
	site, local := config.DeployedWebOrigin, config.LocalWebOrigin
	tests := []struct {
		env, set string
		want     []string
	}{
		{"local", "", []string{site, local}},
		{"test", "", []string{site, local}},
		{"staging", "", []string{site}},
		{"production", "", []string{site}},
		{"production", "https://a.example,https://b.example", []string{"https://a.example", "https://b.example"}},
	}
	for _, tt := range tests {
		cfg := config.Config{Env: config.Env(tt.env), Web: config.Web{AllowedOrigins: tt.set}}
		if got := cfg.WebAllowedOrigins(); !slices.Equal(got, tt.want) {
			t.Errorf("env %s with %q: WebAllowedOrigins() = %v, want %v", tt.env, tt.set, got, tt.want)
		}
	}
}

func TestLoad_refusesAnOriginWithAPathOrAWildcard(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{
		"*", "https://monacolabs.xyz/", "https://monacolabs.xyz/fund", "monacolabs.xyz", "ftp://monacolabs.xyz",
		"https://monacolabs.xyz,", "https://monacolabs.xyz, http://localhost:5173", "://bad",
	} {
		_, err := config.Load(append(required(), "WEB_ALLOWED_ORIGINS="+bad))
		want := "config.Load: invalid_input: invalid WEB_ALLOWED_ORIGINS " +
			"(empty or comma-separated http(s) origins with no path, such as https://monacolabs.xyz)"
		if err == nil || err.Error() != want {
			t.Errorf("WEB_ALLOWED_ORIGINS=%s: Load error = %v, want %q", bad, err, want)
		}
	}
}
