package main

import (
	"context"
	"fmt"
	"slices"

	cabalsqlc "github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	identitysqlc "github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type memberWalletReader interface {
	ListSweepMemberWallets(ctx context.Context) ([]identitysqlc.ListSweepMemberWalletsRow, error)
}

type treasuryWalletReader interface {
	ListTreasuryWallets(ctx context.Context) ([]cabalsqlc.TreasuryWallet, error)
}

type appWalletLister interface {
	ListAppWallets(ctx context.Context) ([]chain.Wallet, error)
}

type walletBook struct {
	members    []chain.Wallet
	treasuries []chain.Wallet
}

func readBook(ctx context.Context, members memberWalletReader, treasuries treasuryWalletReader) (walletBook, error) {
	memberRows, err := members.ListSweepMemberWallets(ctx)
	if err != nil {
		return walletBook{}, fmt.Errorf("user_wallets: %w", err)
	}
	treasuryRows, err := treasuries.ListTreasuryWallets(ctx)
	if err != nil {
		return walletBook{}, fmt.Errorf("treasury_wallets: %w", err)
	}
	var book walletBook
	for _, row := range memberRows {
		book.members = append(book.members, dbWallet(row.PrivyWalletID, row.Address))
	}
	for _, row := range treasuryRows {
		book.treasuries = append(book.treasuries, dbWallet(row.PrivyWalletID, row.Address))
	}
	return book, nil
}

func dbWallet(id, address string) chain.Wallet {
	return chain.Wallet{ID: id, Address: chain.SolanaAddress(address)}
}

func (b walletBook) all() []sweepSource {
	sources := make([]sweepSource, 0, len(b.members)+len(b.treasuries))
	for _, w := range b.members {
		sources = append(sources, sweepSource{kind: kindMember, wallet: w})
	}
	for _, w := range b.treasuries {
		sources = append(sources, sweepSource{kind: kindTreasury, wallet: w})
	}
	return sources
}

func (b walletBook) find(addr chain.SolanaAddress) (sweepSource, bool) {
	for _, src := range b.all() {
		if src.wallet.Address == addr {
			return src, true
		}
	}
	return sweepSource{}, false
}

func loadSweepSources(
	ctx context.Context,
	flags sweepFlags,
	members memberWalletReader,
	treasuries treasuryWalletReader,
	privy appWalletLister,
) ([]sweepSource, string, error) {
	book, err := readBook(ctx, members, treasuries)
	if err != nil {
		return nil, "", err
	}
	switch {
	case len(flags.sources) > 0:
		sources, err := explicitSources(ctx, flags.sources, book, privy)
		if err != nil {
			return nil, "", err
		}
		return sources, fmt.Sprintf("explicit wallets (%d --source)", len(sources)), nil
	case flags.all:
		wallets, err := privy.ListAppWallets(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("privy wallets: %w", err)
		}
		sources := make([]sweepSource, 0, len(wallets))
		for _, w := range wallets {
			src, ok := book.find(w.Address)
			if !ok {
				src.kind = kindPrivy
			}
			src.wallet = w
			sources = append(sources, src)
		}
		return sources, "privy app wallets (--all)", nil
	default:
		return dedupe(book.all()), "postgres user_wallets + treasury_wallets", nil
	}
}

func explicitSources(
	ctx context.Context,
	addresses []chain.SolanaAddress,
	book walletBook,
	privy appWalletLister,
) ([]sweepSource, error) {
	privyWallets := map[chain.SolanaAddress]chain.Wallet{}
	if slices.ContainsFunc(addresses, func(a chain.SolanaAddress) bool { _, ok := book.find(a); return !ok }) {
		wallets, err := privy.ListAppWallets(ctx)
		if err != nil {
			return nil, fmt.Errorf("privy wallets: %w", err)
		}
		for _, w := range wallets {
			privyWallets[w.Address] = w
		}
	}
	sources := make([]sweepSource, 0, len(addresses))
	for _, addr := range addresses {
		src, ok := book.find(addr)
		if !ok {
			src = sweepSource{kind: kindExplicit, wallet: chain.Wallet{Address: addr}}
			if w, found := privyWallets[addr]; found {
				src.wallet = w
			}
		}
		sources = append(sources, src)
	}
	return dedupe(sources), nil
}

func dedupe(sources []sweepSource) []sweepSource {
	seen := map[chain.SolanaAddress]bool{}
	out := make([]sweepSource, 0, len(sources))
	for _, src := range sources {
		if seen[src.wallet.Address] {
			continue
		}
		seen[src.wallet.Address] = true
		out = append(out, src)
	}
	return out
}
