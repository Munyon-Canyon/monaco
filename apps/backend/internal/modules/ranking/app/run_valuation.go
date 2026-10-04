package app

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
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
}

type RunValuation struct {
	ports Ports
	usdc  chain.SolanaAddress
}

func NewRunValuation(ports Ports, usdc chain.SolanaAddress) RunValuation {
	return RunValuation{ports: ports, usdc: usdc}
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
	positions []treasury.CabalPositions
	stakes    []treasury.MemberStake
	paused    funding.PausedSet
	assets    []market.Asset
}

func (r RunValuation) read(ctx context.Context, at time.Time) (readData, error) {
	cabals, err := r.ports.Cabals.AllCabals(ctx)
	if err != nil {
		return readData{}, err
	}
	if _, err := r.ports.Cabals.MembersOf(ctx, cabalIDs(cabals)); err != nil {
		return readData{}, err
	}
	positions, err := r.ports.Treasury.CabalPositionsAt(ctx, at)
	if err != nil {
		return readData{}, err
	}
	stakes, err := r.ports.Treasury.MemberStakesAt(ctx, at)
	if err != nil {
		return readData{}, err
	}
	if err := r.readUsers(ctx, stakes); err != nil {
		return readData{}, err
	}
	paused, err := r.ports.Funding.PausedCabals(ctx)
	if err != nil {
		return readData{}, err
	}
	assets, err := r.ports.Market.ListAll(ctx)
	if err != nil {
		return readData{}, err
	}
	return readData{cabals: cabals, positions: positions, stakes: stakes, paused: paused, assets: assets}, nil
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
	inputs, idsForPrices, err := r.inputs(ctx, data.cabals, data.positions, data.paused, byMint)
	if err != nil {
		return Valuation{}, err
	}
	latest, err := r.ports.Market.LatestPrices(ctx)
	if err != nil {
		return Valuation{}, err
	}
	sessions, err := r.sessions(ctx, idsForPrices, at)
	if err != nil {
		return Valuation{}, err
	}
	atTime, err := r.pricesAtPricingInstants(ctx, idsForPrices, sessions, at)
	if err != nil {
		return Valuation{}, err
	}
	values, flagged, err := r.values(ctx, inputs, byMint, latest, atTime, sessions, data.stakes, at)
	if err != nil {
		return Valuation{}, err
	}
	return Valuation{
		AsOf: at, PricesAsOf: at, Cabals: values, Flagged: flagged, Excluded: len(data.cabals) - len(values),
	}, nil
}

func (r RunValuation) pricesAtPricingInstants(
	ctx context.Context,
	assetIDs []market.AssetID,
	sessions map[market.AssetID]market.SessionInfo,
	at time.Time,
) (map[market.AssetID]market.Price, error) {
	groups := map[int64][]market.AssetID{at.UnixNano(): {}}
	instants := map[int64]time.Time{at.UnixNano(): at}
	for _, assetID := range assetIDs {
		instant := pricingInstant(sessions[assetID], at)
		groups[instant.UnixNano()] = append(groups[instant.UnixNano()], assetID)
		instants[instant.UnixNano()] = instant
	}
	prices := make(map[market.AssetID]market.Price, len(assetIDs))
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		page, err := r.ports.Market.PricesAsOf(ctx, groups[key], instants[key])
		if err != nil {
			return nil, err
		}
		maps.Copy(prices, page)
	}
	return prices, nil
}

func pricingInstant(session market.SessionInfo, at time.Time) time.Time {
	if string(session.State) == "open" || session.Continuous {
		return at
	}
	return session.LastClose
}

func logExcluded(ctx context.Context, cabalID ids.CabalID, code errs.Code) {
	observability.Info(ctx, observability.RankingCabalExcluded,
		slog.String("cabal", cabalID.String()), slog.String("reason", string(code)))
}

func (r RunValuation) sessions(
	ctx context.Context,
	assetIDs []market.AssetID,
	at time.Time,
) (map[market.AssetID]market.SessionInfo, error) {
	sessions := make(map[market.AssetID]market.SessionInfo, len(assetIDs))
	for _, assetID := range assetIDs {
		session, err := r.ports.Market.Session(ctx, assetID, at)
		if err != nil {
			return nil, err
		}
		sessions[assetID] = session
	}
	return sessions, nil
}

func (r RunValuation) inputs(
	ctx context.Context,
	cabals []cabalport.CabalView,
	positions []treasury.CabalPositions,
	paused funding.PausedSet,
	byMint map[string]market.Asset,
) ([]valuationInput, []market.AssetID, error) {
	byCabal := positionsByCabal(positions)
	inputs := make([]valuationInput, 0, len(cabals))
	idsForPrices := []market.AssetID{}
	seenAsset := map[market.AssetID]bool{}
	for _, cabal := range cabals {
		position, found := byCabal[cabal.ID]
		if paused.Global || paused.Cabals[cabal.ID] != nil {
			logExcluded(ctx, cabal.ID, errs.CodeCabalPaused)
		}
		input, assetIDs, ok, err := r.input(cabal, position, found, paused, byMint)
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
	for _, holding := range tokens {
		asset, ok := assets[string(holding.Mint)]
		if !ok {
			return valuationInput{}, nil, false, errs.New(
				errs.CodeUpstreamUnavailable,
				"ranking.RunValuation",
				slog.String("mint", string(holding.Mint)),
			)
		}
		assetIDs = append(assetIDs, asset.ID)
	}
	return valuationInput{cabalID: cabal.ID, position: position, cash: cash, tokens: tokens}, assetIDs, true, nil
}

func (r RunValuation) values(
	ctx context.Context,
	inputs []valuationInput,
	byMint map[string]market.Asset,
	latest, atTime map[market.AssetID]market.Price,
	sessions map[market.AssetID]market.SessionInfo,
	stakes []treasury.MemberStake,
	at time.Time,
) ([]CabalValue, []CabalValue, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	values := make([]CabalValue, 0, len(inputs))
	flagged := []CabalValue{}
	stage := concurrency.Stage(
		ctx,
		concurrency.Feed(ctx, inputs),
		64,
		func(_ context.Context, in valuationInput) (valuedCabal, error) {
			nav, flags, err := r.cabalNAV(
				in.cash,
				in.tokens,
				in.position.TotalShares,
				byMint,
				sessions,
				latest,
				atTime,
				at,
			)
			return valuedCabal{valuationInput: in, nav: nav, flags: flags}, err
		},
	)
	for result := range stage {
		if result.Err != nil {
			return nil, nil, result.Err
		}
		if len(result.Val.flags) != 0 {
			flagged = append(flagged, CabalValue{CabalID: result.Val.cabalID, Flags: result.Val.flags})
			continue
		}
		if err := conservation(
			result.Val.cabalID,
			result.Val.position.TotalShares,
			stakes,
			result.Val.nav.Value,
		); err != nil {
			if errs.CodeOf(err) == errs.CodeConservationBroken {
				logExcluded(ctx, result.Val.cabalID, errs.CodeConservationBroken)
				continue
			}
			return nil, nil, err
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

func (r RunValuation) readUsers(ctx context.Context, stakes []treasury.MemberStake) error {
	userIDs := make([]ids.UserID, 0, len(stakes))
	seen := map[ids.UserID]bool{}
	for _, stake := range stakes {
		if !seen[stake.UserID] {
			seen[stake.UserID] = true
			userIDs = append(userIDs, stake.UserID)
		}
	}
	for len(userIDs) > 0 {
		limit := min(len(userIDs), 500)
		if _, err := r.ports.Users.UsersByID(ctx, userIDs[:limit]); err != nil {
			return err
		}
		userIDs = userIDs[limit:]
	}
	return nil
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
	cash money.Micros,
	positions []treasury.Position,
	totalShares money.SharesUnits,
	assets map[string]market.Asset,
	sessions map[market.AssetID]market.SessionInfo,
	latest, atTime map[market.AssetID]market.Price,
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
		session := sessions[asset.ID]
		price, priceFlags := domain.PriceForBoard(
			domain.Session{
				Open:       string(session.State) == "open",
				Continuous: session.Continuous,
				LastClose:  session.LastClose,
			},
			sample(latest, asset.ID),
			sample(atTime, asset.ID),
			at,
		)
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
	nav, err := domain.CabalNAV(domain.NAVInput{USDC: cash, Holdings: holdings, TotalShares: totalShares})
	return nav, nil, err
}

type valuationInput struct {
	cabalID  ids.CabalID
	position treasury.CabalPositions
	cash     money.Micros
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
