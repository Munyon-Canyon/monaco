package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type WithdrawalStatus string

const (
	WithdrawalCreated   WithdrawalStatus = "created"
	WithdrawalSubmitted WithdrawalStatus = "submitted"
	WithdrawalConfirmed WithdrawalStatus = "confirmed"
	WithdrawalFailed    WithdrawalStatus = "failed"
)

const MinWithdrawalMicros = 1_000_000

type FeeMode string

const (
	FeeIncluded FeeMode = "included"
	FeeOnTop    FeeMode = "on_top"
)

func WithdrawalFee(network chain.Network) (money.Micros, FeeMode) {
	if network.IsEVM() {
		return money.Micros{}, FeeIncluded
	}
	return money.Micros{}, FeeOnTop
}

func ArrivalSeconds(network chain.Network) int {
	if network == chain.NetworkEthereum {
		return 300
	}
	if network.IsEVM() {
		return 120
	}
	return 60
}

func MaxSendable(available, fee money.Micros, mode FeeMode) money.Micros {
	if mode == FeeIncluded {
		return available
	}
	left, err := available.Sub(fee)
	if err != nil {
		return money.Micros{}
	}
	return left
}

type DestinationKind string

const (
	DestinationSolana DestinationKind = "solana"
	DestinationEVM    DestinationKind = "evm"
)

func evmWithdrawalEnabled(n chain.Network) bool {
	return n.IsEVM() && n != chain.NetworkRobinhood
}

type WithdrawalRequest struct {
	Amount  money.Micros
	Network chain.Network
	To      chain.SolanaAddress
	EVM     chain.EVMAddress
}

func (r WithdrawalRequest) Kind() DestinationKind {
	if r.Network.IsEVM() {
		return DestinationEVM
	}
	return DestinationSolana
}

func (r WithdrawalRequest) Address() string {
	if r.Kind() == DestinationEVM {
		return r.EVM.String()
	}
	return string(r.To)
}

type RawWithdrawal struct {
	Amount, Kind, Network, To string
}

func ParseWithdrawalRequest(raw RawWithdrawal) (WithdrawalRequest, error) {
	const op = "funding.ParseWithdrawalRequest"
	amount, err := money.ParseMicros(raw.Amount)
	if err != nil {
		return WithdrawalRequest{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	if amount.Cmp(money.MicrosFromUint64(MinWithdrawalMicros)) < 0 {
		return WithdrawalRequest{}, errs.New(errs.CodeInvalidInput, op,
			slog.String("amount_micros", amount.String()), slog.Int("min_micros", MinWithdrawalMicros))
	}
	request, err := parseDestination(raw, true)
	request.Amount = amount
	return request, err
}

type RawWithdrawalPreview struct {
	Amount            *string
	Kind, Network, To string
}

type WithdrawalPreviewRequest struct {
	Network chain.Network
	Amount  *money.Micros
	EVM     *chain.EVMAddress
}

func ParseWithdrawalPreview(raw RawWithdrawalPreview) (WithdrawalPreviewRequest, error) {
	const op = "funding.ParseWithdrawalPreview"
	var amount *money.Micros
	if raw.Amount != nil {
		parsed, err := money.ParseMicros(*raw.Amount)
		if err != nil {
			return WithdrawalPreviewRequest{}, errs.Wrap(err, errs.CodeInvalidInput, op)
		}
		amount = &parsed
	}
	request, err := parseDestination(RawWithdrawal{Kind: raw.Kind, Network: raw.Network, To: raw.To}, raw.To != "")
	if err != nil {
		return WithdrawalPreviewRequest{}, err
	}
	out := WithdrawalPreviewRequest{Network: request.Network, Amount: amount}
	if request.Kind() == DestinationEVM && raw.To != "" {
		out.EVM = &request.EVM
	}
	return out, nil
}

func parseDestination(raw RawWithdrawal, withAddress bool) (WithdrawalRequest, error) {
	const op = "funding.ParseWithdrawalRequest"
	network, err := chain.ParseNetwork(raw.Network)
	if err != nil {
		return WithdrawalRequest{}, err
	}
	switch DestinationKind(raw.Kind) {
	case "", DestinationSolana:
		if network != chain.NetworkSolana {
			return WithdrawalRequest{}, errs.New(errs.CodeInvalidNetwork, op,
				slog.String("network", string(network)), slog.String("reason", "solana_destination"))
		}
		if !withAddress {
			return WithdrawalRequest{Network: network}, nil
		}
		to, err := parseSolanaDestination(raw.To)
		return WithdrawalRequest{Network: network, To: to}, err
	case DestinationEVM:
		if !evmWithdrawalEnabled(network) {
			return WithdrawalRequest{}, errs.New(errs.CodeInvalidNetwork, op,
				slog.String("network", string(network)), slog.String("reason", "not_enabled"))
		}
		if !withAddress {
			return WithdrawalRequest{Network: network}, nil
		}
		to, err := chain.ParseEVMAddress(raw.To)
		if err != nil {
			return WithdrawalRequest{}, err
		}
		if to == (chain.EVMAddress{}) {
			return WithdrawalRequest{}, errs.New(errs.CodeInvalidAddress, op, slog.String("reason", "zero_address"))
		}
		return WithdrawalRequest{Network: network, EVM: to}, nil
	}
	return WithdrawalRequest{}, errs.New(errs.CodeInvalidInput, op, slog.String("destination_kind", raw.Kind))
}

func parseSolanaDestination(raw string) (chain.SolanaAddress, error) {
	const op = "funding.ParseWithdrawalRequest"
	to, err := chain.ParseAddress(raw)
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInvalidAddress, op)
	}
	if !to.OnCurve() {
		return "", errs.New(errs.CodeInvalidAddress, op, slog.String("reason", "off_curve"))
	}
	return to, nil
}

const (
	WithdrawalNotSent           = "withdrawal_not_sent"
	WithdrawalBlockhashExpired  = "blockhash_expired"
	WithdrawalTransactionFailed = "transaction_failed"
	WithdrawalTransferRejected  = "transfer_rejected"
	WithdrawalTransferFailed    = "transfer_failed"
	WithdrawalSupportResolved   = "support_resolved"
)
