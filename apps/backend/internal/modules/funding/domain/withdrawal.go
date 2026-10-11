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
		to, err := parseSolanaDestination(raw.To)
		return WithdrawalRequest{Amount: amount, Network: network, To: to}, err
	case DestinationEVM:
		if !evmWithdrawalEnabled(network) {
			return WithdrawalRequest{}, errs.New(errs.CodeInvalidNetwork, op,
				slog.String("network", string(network)), slog.String("reason", "not_enabled"))
		}
		to, err := chain.ParseEVMAddress(raw.To)
		if err != nil {
			return WithdrawalRequest{}, err
		}
		if to == (chain.EVMAddress{}) {
			return WithdrawalRequest{}, errs.New(errs.CodeInvalidAddress, op, slog.String("reason", "zero_address"))
		}
		return WithdrawalRequest{Amount: amount, Network: network, EVM: to}, nil
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
)
