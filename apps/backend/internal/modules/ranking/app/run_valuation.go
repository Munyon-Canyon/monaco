package app

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type CabalValue struct {
	CabalID     ids.CabalID
	Value       money.Micros
	NavPerShare money.Micros
	TotalShares money.SharesUnits
	Flags       []domain.Flag
}

type Valuation struct {
	AsOf       time.Time
	PricesAsOf time.Time
	Cabals     []CabalValue
	Flagged    []CabalValue
	Entries    []Entry
	Excluded   int
}

type Entry struct {
	Board             string    `json:"board"`
	Range             string    `json:"range"`
	Rank              int       `json:"rank"`
	SubjectID         uuid.UUID `json:"subject_id"`
	SubjectName       string    `json:"subject_name"`
	SubjectHandle     *string   `json:"subject_handle"`
	SubjectPictureURL *string   `json:"subject_picture_url"`
	SubjectCreatedAt  time.Time `json:"subject_created_at"`
	ValueMicros       int64     `json:"value_micros"`
	PnLMicros         int64     `json:"pnl_micros"`
	ReturnBps         *int64    `json:"return_bps"`
	PricesAsOf        time.Time `json:"prices_as_of"`
	ComputedAt        time.Time `json:"computed_at"`
	Flags             []string  `json:"flags"`
	JoinMode          *string   `json:"join_mode"`
	MemberCount       int       `json:"member_count"`
}

type RunValuation struct {
	ports    Ports
	usdc     chain.SolanaAddress
	unpriced *unpricedLog
}

func NewRunValuation(ports Ports, usdc chain.SolanaAddress) RunValuation {
	return RunValuation{ports: ports, usdc: usdc, unpriced: &unpricedLog{last: map[string]time.Time{}}}
}

func (r RunValuation) Run(ctx context.Context, at time.Time) (Valuation, error) {
	data, err := r.read(ctx, at)
	if err != nil {
		return Valuation{}, err
	}
	return r.value(ctx, data, at)
}

type readData struct {
	cabals    []cabalport.CabalView
	members   map[ids.CabalID][]cabalport.MemberView
	positions []treasury.CabalPositions
	stakes    []treasury.MemberStake
	reserved  map[ids.CabalID]money.Micros
	users     map[ids.UserID]identity.UserCard
	paused    funding.PausedSet
	assets    []market.Asset
	ranged    []rangedBook
	listings  map[market.ListingKey]market.ListingPrice
}

func (r RunValuation) read(ctx context.Context, at time.Time) (readData, error) {
	cabals, err := r.ports.Cabals.AllCabals(ctx)
	if err != nil {
		return readData{}, err
	}
	members, err := r.ports.Cabals.MembersOf(ctx, cabalIDs(cabals))
	if err != nil {
		return readData{}, err
	}
	inputs, err := r.ports.Treasury.ValuationInputsAt(ctx, at)
	if err != nil {
		return readData{}, err
	}
	positions, stakes, reserved := inputs.Positions, inputs.Stakes, inputs.Reserved
	users, err := r.readUsers(ctx, stakes, members)
	if err != nil {
		return readData{}, err
	}
	paused, err := r.ports.Funding.PausedCabals(ctx)
	if err != nil {
		return readData{}, err
	}
	assets, err := r.ports.Market.AssetsByMints(ctx, r.heldMints(positions))
	if err != nil {
		return readData{}, err
	}
	listings, err := r.listingPrices(ctx, positions, at)
	if err != nil {
		return readData{}, err
	}
	ranged, err := r.readRanged(ctx, at)
	if err != nil {
		return readData{}, err
	}
	return readData{
		cabals: cabals, members: members, positions: positions, stakes: stakes, reserved: reserved, users: users,
		paused: paused, assets: assets, ranged: ranged, listings: listings,
	}, nil
}

func (r RunValuation) heldMints(positions []treasury.CabalPositions) []string {
	seen := map[string]bool{}
	mints := []string{}
	for _, position := range positions {
		for _, holding := range position.Holdings {
			mint := string(holding.Mint)
			if holding.Mint != r.usdc && !seen[mint] {
				seen[mint] = true
				mints = append(mints, mint)
			}
		}
	}
	slices.Sort(mints)
	return mints
}

func cabalIDs(cabals []cabalport.CabalView) []ids.CabalID {
	ids := make([]ids.CabalID, len(cabals))
	for i, cabal := range cabals {
		ids[i] = cabal.ID
	}
	return ids
}

func (r RunValuation) value(ctx context.Context, data readData, at time.Time) (Valuation, error) {
	byMint, _ := assetsByMint(data.assets)
	inputs, idsForPrices, err := r.inputs(ctx, data, byMint, at)
	if err != nil {
		return Valuation{}, err
	}
	atTime, err := r.ports.Market.PricesAsOf(ctx, idsForPrices, at)
	if err != nil {
		return Valuation{}, err
	}
	values, flagged, err := r.values(ctx, inputs, byMint, atTime, data.stakes, at)
	if err != nil {
		return Valuation{}, err
	}
	entries, err := r.entries(ctx, data, values, flagged, at)
	if err != nil {
		return Valuation{}, err
	}
	return Valuation{
		AsOf: at, PricesAsOf: at, Cabals: values, Flagged: flagged, Entries: entries,
		Excluded: len(data.cabals) - len(values),
	}, nil
}

func logSkipped(ctx context.Context) func(ids.CabalID) {
	seen := map[ids.CabalID]bool{}
	return func(cabalID ids.CabalID) {
		if !seen[cabalID] {
			seen[cabalID] = true
			observability.Info(ctx, observability.RankingCabalExcluded,
				slog.String("cabal", cabalID.String()), slog.String("reason", "shares_exceed_total"))
		}
	}
}

func logExcluded(ctx context.Context, cabalID ids.CabalID, code errs.Code) {
	observability.Info(ctx, observability.RankingCabalExcluded,
		slog.String("cabal", cabalID.String()), slog.String("reason", string(code)))
}

func (r RunValuation) entries(
	ctx context.Context,
	data readData,
	valued, flagged []CabalValue,
	at time.Time,
) ([]Entry, error) {
	flaggedIDs := make([]ids.CabalID, len(flagged))
	for i, cabal := range flagged {
		flaggedIDs[i] = cabal.CabalID
	}
	previous, fallback, err := r.previousRows(ctx, flaggedIDs, valued)
	if err != nil {
		return nil, err
	}
	views := make(map[ids.CabalID]cabalport.CabalView, len(data.cabals))
	for _, cabal := range data.cabals {
		views[cabal.ID] = cabal
	}
	return buildEntries(boardInput{
		at: at, views: views, members: data.members, stakes: data.stakes, users: data.users,
		valued: valued, flagged: flagged, previous: previous, ranged: data.ranged, skipped: r.logSkipped(ctx),
		fallback: fallback,
	})
}

func (r RunValuation) previousRows(
	ctx context.Context,
	flagged []ids.CabalID,
	valued []CabalValue,
) ([]Entry, map[fallbackKey]Entry, error) {
	flaggedSet := make(map[uuid.UUID]bool, len(flagged))
	for _, id := range flagged {
		flaggedSet[id.UUID()] = true
	}
	all := slices.Clone(flagged)
	for _, cabal := range valued {
		all = append(all, cabal.CabalID)
	}
	rows, err := PreviousEntries(ctx, r.ports.Previous, all)
	if err != nil {
		return nil, nil, err
	}
	previous := []Entry{}
	fallback := map[fallbackKey]Entry{}
	for _, row := range rows {
		switch {
		case row.Board == cabalsBoard && !flaggedSet[row.SubjectID]:
			fallback[fallbackKey{cabal: row.SubjectID, rng: row.Range}] = row
		case row.Board == cabalsBoard || flaggedBoard(row.Board, flaggedSet):
			previous = append(previous, row)
		}
	}
	return previous, fallback, nil
}

func flaggedBoard(board string, flagged map[uuid.UUID]bool) bool {
	for id := range flagged {
		if board == MembersBoard(id) {
			return true
		}
	}
	return false
}

func (RunValuation) logSkipped(ctx context.Context) func(ids.CabalID, Window, string) {
	return func(cabalID ids.CabalID, w Window, reason string) {
		observability.Degraded(ctx, observability.RankingRangeStartSkipped,
			slog.String("cabal", cabalID.String()), slog.String("range", string(w.Range)),
			slog.String("reason", reason))
	}
}

func (r RunValuation) inputs(
	ctx context.Context,
	data readData,
	byMint map[string]market.Asset,
	at time.Time,
) ([]valuationInput, []market.AssetID, error) {
	byCabal := positionsByCabal(data.positions)
	inputs := make([]valuationInput, 0, len(data.cabals))
	idsForPrices := []market.AssetID{}
	seenAsset := map[market.AssetID]bool{}
	for _, cabal := range data.cabals {
		position, found := byCabal[cabal.ID]
		if data.paused.Global || data.paused.Cabals[cabal.ID] != nil {
			logExcluded(ctx, cabal.ID, errs.CodeCabalPaused)
		}
		input, assetIDs, ok, err := r.input(cabal, position, found, data.paused, byMint)
		var miss *unpricedListing
		input, miss = offSolana(input, position, data.listings, at)
		r.unpriced.log(ctx, cabal.ID, miss, at)
		input.reserved = data.reserved[cabal.ID]
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		for _, assetID := range assetIDs {
			if !seenAsset[assetID] {
				seenAsset[assetID] = true
				idsForPrices = append(idsForPrices, assetID)
			}
		}
		inputs = append(inputs, input)
	}
	return inputs, idsForPrices, nil
}

func (r RunValuation) input(
	cabal cabalport.CabalView,
	position treasury.CabalPositions,
	found bool,
	paused funding.PausedSet,
	assets map[string]market.Asset,
) (valuationInput, []market.AssetID, bool, error) {
	if !found || cabal.Status == cabalport.StatusBanned || paused.Global || paused.Cabals[cabal.ID] != nil {
		return valuationInput{}, nil, false, nil
	}
	cash, tokens, err := splitCash(position.Holdings, r.usdc)
	if err != nil {
		return valuationInput{}, nil, false, err
	}
	assetIDs := make([]market.AssetID, 0, len(tokens))
	unpriced := position.Unpriced
	for _, holding := range tokens {
		asset, ok := assets[string(holding.Mint)]
		if !ok {
			unpriced = true
			continue
		}
		assetIDs = append(assetIDs, asset.ID)
	}
	if unpriced {
		return valuationInput{cabalID: cabal.ID, position: position, unpriced: true}, nil, true, nil
	}
	return valuationInput{cabalID: cabal.ID, position: position, cash: cash, tokens: tokens}, assetIDs, true, nil
}

func (r RunValuation) values(
	ctx context.Context,
	inputs []valuationInput,
	byMint map[string]market.Asset,
	atTime map[market.AssetID]market.Price,
	stakes []treasury.MemberStake,
	at time.Time,
) ([]CabalValue, []CabalValue, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	values := make([]CabalValue, 0, len(inputs))
	flagged := []CabalValue{}
	skipped := logSkipped(ctx)
	stage := concurrency.Stage(
		ctx,
		concurrency.Feed(ctx, inputs),
		64,
		func(_ context.Context, in valuationInput) (valuedCabal, error) {
			if in.unpriced {
				return valuedCabal{valuationInput: in, flags: []domain.Flag{domain.FlagUnpricedAssets}}, nil
			}
			nav, flags, err := r.cabalNAV(
				in.cash,
				in.reserved,
				in.tokens,
				in.position.TotalShares,
				byMint,
				atTime,
				at,
			)
			return valuedCabal{valuationInput: in, nav: nav, flags: flags}, err
		},
	)
	for result := range stage {
		if errs.CodeOf(result.Err) == errs.CodeConservationBroken {
			logExcluded(ctx, result.Val.cabalID, errs.CodeConservationBroken)
			continue
		}
		if result.Err != nil {
			return nil, nil, result.Err
		}
		if len(result.Val.flags) != 0 {
			flagged = append(flagged, CabalValue{CabalID: result.Val.cabalID, Flags: result.Val.flags})
			continue
		}
		skip, err := r.unconserved(ctx, result.Val, stakes, skipped)
		if err != nil {
			return nil, nil, err
		}
		if skip {
			continue
		}
		values = append(
			values,
			CabalValue{
				CabalID:     result.Val.cabalID,
				Value:       result.Val.nav.Value,
				NavPerShare: result.Val.nav.PerShare,
				TotalShares: result.Val.position.TotalShares,
			},
		)
	}
	return values, flagged, nil
}

func (RunValuation) unconserved(
	ctx context.Context, cabal valuedCabal, stakes []treasury.MemberStake, skipped func(ids.CabalID),
) (bool, error) {
	err := conservation(cabal.cabalID, cabal.position.TotalShares, stakes, cabal.nav.Value)
	switch {
	case err == nil:
		return false, nil
	case errs.CodeOf(err) == errs.CodeConservationBroken:
		logExcluded(ctx, cabal.cabalID, errs.CodeConservationBroken)
		return true, nil
	case errors.Is(err, domain.ErrUnusableStart):
		skipped(cabal.cabalID)
		return true, nil
	}
	return false, err
}

func (r RunValuation) readUsers(
	ctx context.Context,
	stakes []treasury.MemberStake,
	members map[ids.CabalID][]cabalport.MemberView,
) (map[ids.UserID]identity.UserCard, error) {
	userIDs := make([]ids.UserID, 0, len(stakes))
	seen := map[ids.UserID]bool{}
	add := func(id ids.UserID) {
		if !seen[id] {
			seen[id] = true
			userIDs = append(userIDs, id)
		}
	}
	for _, stake := range stakes {
		add(stake.UserID)
	}
	for _, list := range members {
		for _, member := range list {
			add(member.UserID)
		}
	}
	users := make(map[ids.UserID]identity.UserCard, len(userIDs))
	for len(userIDs) > 0 {
		limit := min(len(userIDs), 500)
		page, err := r.ports.Users.UsersByID(ctx, userIDs[:limit])
		if err != nil {
			return nil, err
		}
		maps.Copy(users, page)
		userIDs = userIDs[limit:]
	}
	return users, nil
}

func assetsByMint(assets []market.Asset) (map[string]market.Asset, []market.AssetID) {
	byMint := make(map[string]market.Asset, len(assets))
	ids := make([]market.AssetID, 0, len(assets))
	for _, asset := range assets {
		byMint[asset.Mint.String()] = asset
		ids = append(ids, asset.ID)
	}
	return byMint, ids
}

func positionsByCabal(in []treasury.CabalPositions) map[ids.CabalID]treasury.CabalPositions {
	out := make(map[ids.CabalID]treasury.CabalPositions, len(in))
	for _, position := range in {
		out[position.CabalID] = position
	}
	return out
}

func (r RunValuation) cabalNAV(
	cash, reserved money.Micros,
	positions []treasury.Position,
	totalShares money.SharesUnits,
	assets map[string]market.Asset,
	atTime map[market.AssetID]market.Price,
	at time.Time,
) (domain.NAV, []domain.Flag, error) {
	holdings := make([]domain.Holding, 0, len(positions))
	flags := []domain.Flag{}
	for _, holding := range positions {
		asset, ok := assets[string(holding.Mint)]
		if !ok {
			return domain.NAV{}, nil, errs.New(
				errs.CodeUpstreamUnavailable,
				"ranking.RunValuation",
				slog.String("mint", string(holding.Mint)),
			)
		}
		price, priceFlags := domain.PriceForBoard(sample(atTime, asset.ID), at)
		flags = append(flags, priceFlags...)
		if len(priceFlags) != 0 {
			continue
		}
		holdings = append(
			holdings,
			domain.Holding{AssetID: asset.ID.String(), Units: holding.Units, Price: price.Price},
		)
	}
	if len(flags) != 0 {
		return domain.NAV{}, flags, nil
	}
	nav, err := domain.CabalNAV(
		domain.NAVInput{USDC: cash, Reserved: reserved, Holdings: holdings, TotalShares: totalShares},
	)
	return nav, nil, err
}

type valuationInput struct {
	unpriced bool
	cabalID  ids.CabalID
	position treasury.CabalPositions
	cash     money.Micros
	reserved money.Micros
	tokens   []treasury.Position
}

type valuedCabal struct {
	valuationInput
	nav   domain.NAV
	flags []domain.Flag
}

func splitCash(holdings []treasury.Position, usdc chain.SolanaAddress) (money.Micros, []treasury.Position, error) {
	var cash money.Micros
	tokens := make([]treasury.Position, 0, len(holdings))
	for _, holding := range holdings {
		if holding.Mint != usdc {
			tokens = append(tokens, holding)
			continue
		}
		if holding.Units.Decimals() != 6 {
			return money.Micros{}, nil, errs.New(errs.CodeDecodeFailed, "ranking.RunValuation")
		}
		var err error
		if cash, err = cash.Add(money.MicrosFromUint64(holding.Units.Uint64())); err != nil {
			return money.Micros{}, nil, err
		}
	}
	return cash, tokens, nil
}

func sample(prices map[market.AssetID]market.Price, id market.AssetID) *domain.Sample {
	price, ok := prices[id]
	if !ok {
		return nil
	}
	return &domain.Sample{Price: price.Micros, At: price.ObservedAt}
}

func conservation(cabalID ids.CabalID, total money.SharesUnits, stakes []treasury.MemberStake, nav money.Micros) error {
	equities := []money.Micros{}
	for _, stake := range stakes {
		if stake.CabalID != cabalID {
			continue
		}
		equity, err := domain.MemberEquity(stake.ShareUnits, total, nav)
		if err != nil {
			return err
		}
		equities = append(equities, equity)
	}
	return domain.CheckConservation(nav, equities)
}
