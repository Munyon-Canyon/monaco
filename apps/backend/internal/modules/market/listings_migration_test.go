package market_test

import (
	"encoding/csv"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const linkMigration = "../../../migrations/20261011063200_market_listings_link.sql"

type datasetRow struct {
	issuer, symbol, network, address, decimals string
	enabled                                    bool
}

func dataset(t *testing.T) []datasetRow {
	t.Helper()
	f, err := os.Open("testdata/listings.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	col := map[string]int{}
	for i, name := range records[0] {
		col[name] = i
	}
	rows := make([]datasetRow, 0, len(records)-1)
	for _, r := range records[1:] {
		address := r[col["address"]]
		if r[col["network"]] != "solana" {
			address = strings.ToLower(address)
		}
		rows = append(rows, datasetRow{
			issuer: r[col["issuer"]], symbol: r[col["symbol"]], network: r[col["network"]], address: address,
			decimals: r[col["decimals"]],
			enabled:  r[col["verified"]] == "True" && r[col["trading_halted"]] == "False",
		})
	}
	return rows
}

func TestAssetListingsHoldEveryVerifiedDatasetRow(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	want := dataset(t)
	if len(want) != 4849 {
		t.Fatalf("dataset rows = %d, want 4849", len(want))
	}
	rows, err := pool.Query(t.Context(), `SELECT issuer, symbol, network, token_address,
		COALESCE(decimals::text, ''), enabled FROM asset_listings`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]datasetRow{}
	for rows.Next() {
		var r datasetRow
		if err := rows.Scan(&r.issuer, &r.symbol, &r.network, &r.address, &r.decimals, &r.enabled); err != nil {
			t.Fatal(err)
		}
		got[r.issuer+"/"+r.network+"/"+r.address] = r
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("asset_listings rows = %d, want %d", len(got), len(want))
	}
	for _, w := range want {
		if g, ok := got[w.issuer+"/"+w.network+"/"+w.address]; !ok || g != w {
			t.Errorf("listing %s/%s/%s = %+v, want %+v", w.issuer, w.network, w.address, g, w)
		}
	}
}

func TestListingLinkBackfillsSolanaPositionsAndRerunsCleanly(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	const mint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	assetID := insertAsset(t, pool, "AAPLx", mint)
	cabalID := ids.Real{}.NewV7()
	execListingSQL(t, pool, `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
		VALUES ('`+cabalID.String()+`', '`+mint+`', 100, 100, now()),
		('`+cabalID.String()+`', 'usdc', 100, 100, now())`)
	migration, err := os.ReadFile(linkMigration)
	if err != nil {
		t.Fatal(err)
	}
	execListingSQL(t, pool, string(migration), string(migration))
	var linked int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM asset_listings
		WHERE asset_id = $1 AND issuer = 'xstocks'`, assetID).Scan(&linked); err != nil || linked < 2 {
		t.Fatalf("xStocks listings linked to AAPLx = %d, %v, want its Solana and EVM listings", linked, err)
	}
	var network, token string
	if err := pool.QueryRow(t.Context(), `SELECT l.network, l.token_address FROM cabal_positions AS p
		JOIN asset_listings AS l ON l.id = p.listing_id WHERE p.cabal_id = $1`, cabalID).Scan(&network, &token); err != nil ||
		network != "solana" || token != mint {
		t.Fatalf("position listing = %s %s, %v, want the Solana listing of %s", network, token, err, mint)
	}
}

func TestListingLinkRefusesAMintThatDiffersFromTheDataset(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	insertAsset(t, pool, "TSLAx", "So11111111111111111111111111111111111111112")
	migration, err := os.ReadFile(linkMigration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), string(migration)); err == nil ||
		!strings.Contains(err.Error(), "Solana mints differ from assets: TSLAx") {
		t.Fatalf("link migration error = %v, want the TSLAx mismatch", err)
	}
}

func insertAsset(t *testing.T, pool *pgxpool.Pool, symbol, mint string) uuid.UUID {
	t.Helper()
	id := ids.Real{}.NewV7()
	if _, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, $2, $3, 8, 'xstocks', 'equity', $2, true, $2, now(), now())`, id, symbol, mint); err != nil {
		t.Fatal(err)
	}
	return id
}

func execListingSQL(t *testing.T, pool *pgxpool.Pool, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := pool.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
}
