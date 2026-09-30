package domain

import (
	"log/slog"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type asset struct{}

type AssetID = ids.ID[asset]

func NewAssetID(g ids.Generator) AssetID { return ids.New[asset](g) }

func ParseAssetID(raw string) (AssetID, error) { return ids.Parse[asset](raw) }

type Issuer string

const (
	IssuerXStocks   Issuer = "xstocks"
	IssuerTessera   Issuer = "tessera"
	IssuerPreStocks Issuer = "prestocks"
)

func ParseIssuer(raw string) (Issuer, error) {
	switch i := Issuer(raw); i {
	case IssuerXStocks, IssuerTessera, IssuerPreStocks:
		return i, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "market.ParseIssuer", slog.String("issuer", raw))
}

type Kind string

const (
	KindEquity Kind = "equity"
	KindPreIPO Kind = "pre_ipo"
)

func ParseKind(raw string) (Kind, error) {
	switch k := Kind(raw); k {
	case KindEquity, KindPreIPO:
		return k, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "market.ParseKind", slog.String("kind", raw))
}

type Mint struct{ address chain.SolanaAddress }

func ParseMint(raw string) (Mint, error) {
	a, err := chain.ParseAddress(raw)
	if err != nil {
		return Mint{}, err
	}
	return Mint{address: a}, nil
}

func (m Mint) Address() chain.SolanaAddress { return m.address }

func (m Mint) String() string { return string(m.address) }

type Override string

const (
	OverrideAuto Override = "auto"
	OverrideOn   Override = "on"
	OverrideOff  Override = "off"
)

func ParseOverride(raw string) (Override, error) {
	switch o := Override(raw); o {
	case OverrideAuto, OverrideOn, OverrideOff:
		return o, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "market.ParseOverride", slog.String("override", raw))
}

type Multiplier struct {
	Num, Den int64
}

type Asset struct {
	ID             AssetID
	Symbol         string
	Mint           Mint
	Decimals       uint8
	Issuer         Issuer
	Kind           Kind
	DisplayName    string
	LogoURL        string
	UIMultiplier   Multiplier
	IssuerTradable bool
	Override       Override
	PopularRank    int16
	CompanyKey     string
	FirstSeenAt    time.Time
	UpdatedAt      time.Time
}

func (a Asset) Tradable() bool {
	switch a.Override {
	case OverrideOn:
		return true
	case OverrideOff:
		return false
	case OverrideAuto:
	}
	return a.IssuerTradable
}

func CompanyKey(displayName string) string {
	key := strings.ToLower(strings.TrimSpace(displayName))
	for _, suffix := range issuerSuffixes() {
		key = strings.TrimSpace(strings.TrimSuffix(key, suffix))
	}
	return key
}

func issuerSuffixes() []string { return []string{"xstock"} }
