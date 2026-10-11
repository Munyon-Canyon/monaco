package adapters_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coinbaseb20"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/ondo"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/robinhood"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/xstocks"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type fixture struct{ path, file string }

func recordedIssuer(t *testing.T, fixtures ...fixture) *httpclient.Client {
	t.Helper()
	bodies := map[string][]byte{}
	for _, f := range fixtures {
		body, err := os.ReadFile(f.file)
		if err != nil {
			t.Fatal(err)
		}
		bodies[f.path] = body
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if page := r.URL.Query().Get("page"); page != "" {
			key += "?page=" + page
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bodies[key])
	}))
	t.Cleanup(srv.Close)
	return httpclient.New("issuer", httpclient.WithBaseURL(srv.URL), httpclient.WithTimeout(10*time.Second))
}

type storedDecimals struct{}

func (storedDecimals) Verify(_ context.Context, l app.ListingToVerify) (uint8, error) {
	if l.Decimals != nil {
		return *l.Decimals, nil
	}
	return 18, nil
}

type csvListing struct {
	decimals, shares, wrapper string
}

func gistListings(t *testing.T) map[string]csvListing {
	t.Helper()
	f, err := os.Open("../testdata/listings.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]csvListing{}
	for _, r := range records[1:] {
		address := r[6]
		if r[4] != "solana" {
			address = strings.ToLower(address)
		}
		out[r[0]+" "+r[4]+" "+address] = csvListing{decimals: r[7], shares: r[8], wrapper: strings.ToLower(r[9])}
	}
	return out
}

func sameDecimal(a, b string) bool {
	x, okX := new(big.Rat).SetString(a)
	y, okY := new(big.Rat).SetString(b)
	return okX && okY && x.Cmp(y) == 0
}

type storedListing struct {
	key, wrapper, shares string
	decimals             int16
}

func storedListings(t *testing.T, pool sqlc.DBTX) []storedListing {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT issuer || ' ' || network || ' ' || token_address,
		coalesce(wrapper_address, ''), shares_per_token::text, decimals FROM asset_listings`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []storedListing
	for rows.Next() {
		var l storedListing
		if err := rows.Scan(&l.key, &l.wrapper, &l.shares, &l.decimals); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func gistMismatch(want csvListing, got storedListing) string {
	switch {
	case want.wrapper != got.wrapper:
		return fmt.Sprintf("wrapper = %q, want %q", got.wrapper, want.wrapper)
	case want.decimals != "" && want.decimals != strconv.Itoa(int(got.decimals)):
		return fmt.Sprintf("decimals = %d, want %s", got.decimals, want.decimals)
	case want.shares != "" && !sameDecimal(want.shares, got.shares):
		return fmt.Sprintf("shares per token = %s, want %s", got.shares, want.shares)
	}
	return ""
}

func TestListingSync_storesWhatTheIssuersPublishAsTheGistRecordedIt(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `DELETE FROM asset_listings`); err != nil {
		t.Fatal(err)
	}
	sources := []app.ListingSource{
		xstocks.ListingFeed{Client: xstocks.New(recordedIssuer(t,
			fixture{"/api/v2/public/assets?page=0", "xstocks/testdata/assets_page0.json"},
			fixture{"/api/v2/public/assets?page=1", "xstocks/testdata/assets_page1.json"}))},
		ondo.New(recordedIssuer(t,
			fixture{"/api/v2/assets", "ondo/testdata/assets.json"},
			fixture{"/assets/aaplon", "ondo/testdata/aaplon.html"},
			fixture{"/assets/tslaon", "ondo/testdata/tslaon.html"})),
		coinbaseb20.New(
			recordedIssuer(t, fixture{"/v1/tokenized-stocks", "coinbaseb20/testdata/tokenized-stocks.json"}),
		),
		robinhood.New(recordedIssuer(t, fixture{"/rhj/assets", "robinhood/testdata/assets.json"})),
	}
	g := testkit.NewIDs(4969)
	clk := testkit.NewClock(time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC))
	sync := app.NewListingSync(db.New(pool, g, clk), g, clk, sources, storedDecimals{})
	if _, err := sync.Run(t.Context()); err != nil {
		t.Fatal(err)
	}

	gist := gistListings(t)
	seen := map[string]int{}
	for _, l := range storedListings(t, pool) {
		seen[strings.Fields(l.key)[0]]++
		want, ok := gist[l.key]
		if !ok {
			t.Errorf("%s is not in the gist dataset", l.key)
			continue
		}
		if diff := gistMismatch(want, l); diff != "" {
			t.Errorf("%s %s", l.key, diff)
		}
	}
	for _, issuer := range []string{"xstocks", "ondo", "coinbase", "robinhood"} {
		if seen[issuer] == 0 {
			t.Errorf("no %s listings stored: %v", issuer, seen)
		}
	}
}
