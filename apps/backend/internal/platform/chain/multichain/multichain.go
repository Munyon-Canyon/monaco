package multichain

import (
	"context"
	"database/sql/driver"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type stock struct{}

type StockID = ids.ID[stock]

type ListingID uuid.UUID

func ListingIDFrom(u uuid.UUID) ListingID { return ListingID(u) }

func (id ListingID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id ListingID) String() string { return uuid.UUID(id).String() }

func (id ListingID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id ListingID) MarshalText() ([]byte, error) {
	if id.IsZero() {
		return nil, errs.New(errs.CodeInternal, "multichain.ListingID.MarshalText")
	}
	return []byte(id.String()), nil
}

func (id *ListingID) UnmarshalText(text []byte) error {
	u, err := uuid.ParseBytes(text)
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, "multichain.ListingID", slog.String("raw", string(text)))
	}
	*id = ListingID(u)
	return nil
}

func (id ListingID) Value() (driver.Value, error) {
	if id.IsZero() {
		return nil, errs.New(errs.CodeInternal, "multichain.ListingID.Value")
	}
	return id.String(), nil
}

func (id *ListingID) Scan(src any) error {
	if src == nil {
		return errs.New(errs.CodeDecodeFailed, "multichain.ListingID.Scan", slog.String("value", "NULL"))
	}
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, "multichain.ListingID.Scan")
	}
	*id = ListingID(u)
	return nil
}

const (
	MaxImpactBps       = 300
	ListingPriceMaxAge = 5 * time.Minute
)

type Venue string

const (
	VenueJupiter Venue = "jupiter"
	VenueUniswap Venue = "uniswap"
)

type Listing struct {
	ID             ListingID     `json:"id"`
	StockID        StockID       `json:"stock_id"`
	Issuer         string        `json:"issuer"`
	Symbol         string        `json:"symbol"`
	Network        chain.Network `json:"network"`
	TokenAddress   string        `json:"token_address"`
	WrapperAddress string        `json:"wrapper_address,omitempty"`
	Decimals       uint8         `json:"decimals"`
	SharesPerToken *big.Rat      `json:"shares_per_token"`
	Venue          Venue         `json:"venue"`
	Enabled        bool          `json:"enabled"`
}

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type CostKind string

const (
	CostPriceImpact CostKind = "price_impact"
	CostPoolFee     CostKind = "pool_fee"
	CostBridge      CostKind = "bridge"
	CostGas         CostKind = "gas"
	CostPrivyFee    CostKind = "privy_fee"
)

type Cost struct {
	Kind   CostKind     `json:"kind"`
	Micros money.Micros `json:"micros"`
}

type ShareNanos uint64

type Quote struct {
	ListingID   ListingID     `json:"listing_id"`
	Issuer      string        `json:"issuer"`
	Network     chain.Network `json:"network"`
	Venue       Venue         `json:"venue"`
	Side        Side          `json:"side"`
	InputMicros money.Micros  `json:"input_micros"`
	GrossOutput *big.Int      `json:"gross_output_units"`
	Costs       []Cost        `json:"costs"`
	NetShares   ShareNanos    `json:"net_share_nanos"`
	NetMicros   money.Micros  `json:"net_micros"`
	QuotedAt    time.Time     `json:"quoted_at"`
	ExpiresAt   time.Time     `json:"expires_at"`
	RouteID     string        `json:"route_id"`
	Options     RouteOptions  `json:"route_options,omitzero"`
}

const NoHooks = "V4_NO_HOOKS"

type RouteOptions struct {
	HooksOptions string `json:"hooks_options,omitempty"`
}

type UnavailableReason string

const (
	UnavailableNoRoute          UnavailableReason = "no_route"
	UnavailableStale            UnavailableReason = "stale"
	UnavailableExpired          UnavailableReason = "expired"
	UnavailableSimulationFailed UnavailableReason = "simulation_failed"
	UnavailableImpact           UnavailableReason = "impact"
)

type UnavailableVenue struct {
	ListingID ListingID         `json:"listing_id"`
	Network   chain.Network     `json:"network"`
	Venue     Venue             `json:"venue"`
	Reason    UnavailableReason `json:"reason"`
	Detail    string            `json:"reason_detail,omitempty"`
}

type Comparison struct {
	StockID     StockID            `json:"stock_id"`
	Side        Side               `json:"side"`
	Ranked      []Quote            `json:"ranked"`
	Winner      ListingID          `json:"winner"`
	Unavailable []UnavailableVenue `json:"unavailable,omitempty"`
}

func (c Comparison) Pin(n chain.Network) (Comparison, bool) {
	for _, q := range c.Ranked {
		if q.Network == n {
			c.Winner = q.ListingID
			return c, true
		}
	}
	return c, false
}

type Price struct {
	PerShareMicros money.Micros `json:"per_share_micros"`
	AsOf           time.Time    `json:"as_of"`
}

type ActionID string

type ActionState string

const (
	ActionPending   ActionState = "pending"
	ActionSucceeded ActionState = "succeeded"
	ActionRejected  ActionState = "rejected"
	ActionFailed    ActionState = "failed"
)

type StepState string

const (
	StepPending   StepState = "pending"
	StepConfirmed StepState = "confirmed"
	StepFailed    StepState = "failed"
)

type ActionStep struct {
	Network chain.Network `json:"network"`
	TxHash  string        `json:"tx_hash,omitempty"`
	State   StepState     `json:"state"`
}

type ActionStatus struct {
	ID        ActionID     `json:"id"`
	State     ActionState  `json:"state"`
	Steps     []ActionStep `json:"steps"`
	Delivered money.Micros `json:"delivered"`
}

type TransferSpec struct {
	From         chain.Network `json:"from"`
	To           chain.Network `json:"to"`
	Destination  string        `json:"destination"`
	AmountMicros money.Micros  `json:"amount_micros"`
}

type WalletActions interface {
	Transfer(ctx context.Context, walletID string, spec TransferSpec, idemKey string) (ActionID, error)
	Action(ctx context.Context, walletID string, id ActionID) (ActionStatus, error)
}

type EVMWallet struct {
	ID      string           `json:"id"`
	Address chain.EVMAddress `json:"address"`
}

type EVMTransaction struct {
	Network        chain.Network    `json:"network"`
	To             chain.EVMAddress `json:"to"`
	Data           []byte           `json:"data"`
	Value          *big.Int         `json:"value"`
	IdempotencyKey string           `json:"idempotency_key"`
}

type EVMWallets interface {
	CreateAppEVMWallet(ctx context.Context, key string) (EVMWallet, error)
	SignEVMTransaction(ctx context.Context, walletID string, tx EVMTransaction, sponsor bool) ([]byte, error)
	SendEVMTransaction(ctx context.Context, walletID string, tx EVMTransaction, sponsor bool) (string, error)
}

type AutomationID string

type Execution struct {
	ID           string        `json:"id"`
	ActionID     ActionID      `json:"action_id"`
	Network      chain.Network `json:"network"`
	SourceTx     string        `json:"source_tx"`
	AmountMicros money.Micros  `json:"amount_micros"`
	InputMicros  money.Micros  `json:"input_micros"`
	State        ActionState   `json:"state"`
	Signature    string        `json:"solana_signature,omitempty"`
}

type ExecutionPage struct {
	Executions []Execution `json:"executions"`
	Next       string      `json:"next,omitempty"`
}

type DepositForwarding interface {
	CreateAutomation(ctx context.Context) (AutomationID, error)
	Attach(ctx context.Context, evmWalletID string, solanaDest chain.SolanaAddress) error
	Executions(ctx context.Context, walletID, cursor string) (ExecutionPage, error)
	Reindex(ctx context.Context, walletID string, network chain.Network) error
}

type Quoter interface {
	Quote(ctx context.Context, l Listing, side Side, amountMicros money.Micros) (Quote, error)
}

type Router interface {
	Best(ctx context.Context, stockID StockID, side Side, amountMicros money.Micros) (Comparison, error)
}

type NetworkRouter interface {
	BestOn(
		ctx context.Context, stockID StockID, side Side, amountMicros money.Micros, n chain.Network,
	) (Comparison, error)
}

func BestOn(
	ctx context.Context, r Router, stockID StockID, side Side, amountMicros money.Micros, n chain.Network,
) (Comparison, error) {
	if on, ok := r.(NetworkRouter); ok {
		return on.BestOn(ctx, stockID, side, amountMicros, n)
	}
	return r.Best(ctx, stockID, side, amountMicros)
}

type EVMPriceSource interface {
	Prices(ctx context.Context, listings []Listing) (map[ListingID]Price, error)
}

type Pending struct{ Ticket string }

func (p Pending) notImplemented(method string) error {
	return errs.New(errs.CodeNotImplemented, "multichain."+method, slog.String("ticket", p.Ticket))
}

func (p Pending) Transfer(context.Context, string, TransferSpec, string) (ActionID, error) {
	return "", p.notImplemented("Transfer")
}

func (p Pending) Action(context.Context, string, ActionID) (ActionStatus, error) {
	return ActionStatus{}, p.notImplemented("Action")
}

func (p Pending) CreateAppEVMWallet(context.Context, string) (EVMWallet, error) {
	return EVMWallet{}, p.notImplemented("CreateAppEVMWallet")
}

func (p Pending) SignEVMTransaction(context.Context, string, EVMTransaction, bool) ([]byte, error) {
	return nil, p.notImplemented("SignEVMTransaction")
}

func (p Pending) SendEVMTransaction(context.Context, string, EVMTransaction, bool) (string, error) {
	return "", p.notImplemented("SendEVMTransaction")
}

func (p Pending) CreateAutomation(context.Context) (AutomationID, error) {
	return "", p.notImplemented("CreateAutomation")
}

func (p Pending) Attach(context.Context, string, chain.SolanaAddress) error {
	return p.notImplemented("Attach")
}

func (p Pending) Executions(context.Context, string, string) (ExecutionPage, error) {
	return ExecutionPage{}, p.notImplemented("Executions")
}

func (p Pending) Reindex(context.Context, string, chain.Network) error {
	return p.notImplemented("Reindex")
}

func (p Pending) Quote(context.Context, Listing, Side, money.Micros) (Quote, error) {
	return Quote{}, p.notImplemented("Quote")
}

func (p Pending) Best(context.Context, StockID, Side, money.Micros) (Comparison, error) {
	return Comparison{}, p.notImplemented("Best")
}

func (p Pending) Prices(context.Context, []Listing) (map[ListingID]Price, error) {
	return nil, p.notImplemented("Prices")
}
