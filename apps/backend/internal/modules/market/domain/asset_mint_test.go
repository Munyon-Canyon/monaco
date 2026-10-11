package domain_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	aaplxMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	aaplcBase = "0xb200000000000000000000c2e324d24d7eecd1fb"
)

func TestParseMintKeepsASolanaMintUnchanged(t *testing.T) {
	t.Parallel()
	m, err := domain.ParseMint(aaplxMint)
	if err != nil {
		t.Fatal(err)
	}
	if m.String() != aaplxMint || string(m.Address()) != aaplxMint {
		t.Fatalf("mint = %q", m)
	}
	if _, _, listed := m.Listing(); listed {
		t.Fatal("a Solana mint reads as a listing")
	}
}

func TestParseMintNormalizesAnEVMListingToLowercase(t *testing.T) {
	t.Parallel()
	address, err := chain.ParseEVMAddress(aaplcBase)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"base:" + aaplcBase, "base:" + address.Checksum()} {
		m, err := domain.ParseMint(raw)
		if err != nil {
			t.Fatalf("ParseMint(%q) = %v", raw, err)
		}
		network, token, listed := m.Listing()
		if m.String() != "base:"+aaplcBase || network != chain.NetworkBase || token != aaplcBase || !listed {
			t.Fatalf("ParseMint(%q) = %q (%s, %s, %v)", raw, m, network, token, listed)
		}
	}
}

func TestParseMintRejectsABadListing(t *testing.T) {
	t.Parallel()
	address, err := chain.ParseEVMAddress(aaplcBase)
	if err != nil {
		t.Fatal(err)
	}
	badChecksum := "0x" + strings.ToUpper(address.Checksum()[2:4]) + address.Checksum()[4:]
	for _, raw := range []string{
		"solana:" + aaplxMint, "solana:" + aaplcBase, "moon:" + aaplcBase, "base:" + badChecksum, "base:0x12",
	} {
		if _, err := domain.ParseMint(raw); err == nil || errs.CodeOf(err) == errs.CodeInternal {
			t.Errorf("ParseMint(%q) = %v, want a parse error", raw, err)
		}
	}
}
