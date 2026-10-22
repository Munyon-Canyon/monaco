package privy

import (
	"cmp"
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

var _ multichain.WalletActions = (*Client)(nil)

type transferAsset struct {
	Asset   string `json:"asset,omitempty"`
	Chain   string `json:"chain,omitempty"`
	Address string `json:"address,omitempty"`
}

type transferBody struct {
	Amount      string        `json:"amount"`
	AmountType  string        `json:"amount_type"`
	Source      transferAsset `json:"source"`
	Destination transferAsset `json:"destination"`
}

type stepWire struct {
	Type                  string `json:"type"`
	Status                string `json:"status"`
	CAIP2                 string `json:"caip2"`
	TransactionHash       string `json:"transaction_hash"`
	BundleTransactionHash string `json:"bundle_transaction_hash"`
	TransactionSignature  string `json:"transaction_signature"`
}

type actionWire struct {
	ID                string     `json:"id"`
	Status            string     `json:"status"`
	Steps             []stepWire `json:"steps"`
	DestinationAmount string     `json:"destination_amount"`
}

func (c *Client) Transfer(
	ctx context.Context, walletID string, spec multichain.TransferSpec, idemKey string,
) (multichain.ActionID, error) {
	const op = "privy.Transfer"
	if c.authKey == nil {
		return "", errs.New(errs.CodeInternal, op, slog.String("reason", "authorization key missing"))
	}
	if idemKey == "" || spec.AmountMicros.IsZero() {
		return "", errs.New(errs.CodeInvalidInput, op, slog.String("reason", "idempotency key and amount"))
	}
	dest, err := destination(spec.To, spec.Destination)
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	from, okFrom := cashAsset(spec.From)
	to, okTo := cashAsset(spec.To)
	if !okFrom || !okTo {
		return "", errs.New(errs.CodeInvalidNetwork, op,
			slog.String("from", string(spec.From)), slog.String("to", string(spec.To)))
	}
	body := transferBody{
		Amount:      decimalUSDC(spec.AmountMicros),
		AmountType:  "exact_input",
		Source:      transferAsset{Asset: from, Chain: spec.From.PrivyChain()},
		Destination: transferAsset{Asset: to, Chain: spec.To.PrivyChain(), Address: dest},
	}
	var w actionWire
	path := "/v1/wallets/" + url.PathEscape(walletID) + "/transfer"
	if err := c.do(ctx, c.signedPost(op, path, body, idemKey), &w); err != nil {
		return "", err
	}
	status, err := decodeAction(w)
	if err != nil {
		return "", errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("wallet_id", walletID))
	}
	return status.ID, nil
}

func (c *Client) Action(ctx context.Context, walletID string, id multichain.ActionID) (multichain.ActionStatus, error) {
	const op = "privy.Action"
	path := "/v1/wallets/" + url.PathEscape(walletID) + "/actions/" + url.PathEscape(string(id)) + "?include=steps"
	var w actionWire
	if err := c.do(ctx, call{op: op, method: http.MethodGet, path: path}, &w); err != nil {
		return multichain.ActionStatus{}, err
	}
	status, err := decodeAction(w)
	if err != nil {
		return multichain.ActionStatus{}, errs.Wrap(
			err,
			errs.CodeDecodeFailed,
			op,
			slog.String("action_id", string(id)),
		)
	}
	return status, nil
}

func cashAsset(n chain.Network) (string, bool) {
	switch n.Cash().Symbol {
	case "USDC":
		return "usdc", true
	case "USDG":
		return "usdg", true
	}
	return "", false
}

func destination(n chain.Network, raw string) (string, error) {
	if n.IsEVM() {
		addr, err := chain.ParseEVMAddress(raw)
		return addr.String(), err
	}
	addr, err := chain.ParseAddress(raw)
	return string(addr), err
}

func decimalUSDC(m money.Micros) string {
	whole, frac := m.Uint64()/1_000_000, m.Uint64()%1_000_000
	if frac == 0 {
		return strconv.FormatUint(whole, 10)
	}
	digits := strings.TrimRight(strconv.FormatUint(frac+1_000_000, 10)[1:], "0")
	return strconv.FormatUint(whole, 10) + "." + digits
}

func microsOfDecimal(raw string) (money.Micros, error) {
	whole, frac, _ := strings.Cut(raw, ".")
	if len(frac) > 6 {
		return money.Micros{}, errs.New(errs.CodeInvalidInput, "privy.microsOfDecimal", slog.String("raw", raw))
	}
	frac = (frac + "000000")[:6]
	return money.ParseMicros(cmp.Or(strings.TrimLeft(whole+frac, "0"), "0"))
}

func actionState(raw string) (multichain.ActionState, bool) {
	switch state := multichain.ActionState(raw); state {
	case multichain.ActionPending, multichain.ActionSucceeded, multichain.ActionRejected, multichain.ActionFailed:
		return state, true
	}
	return "", false
}

func stepState(raw string) (multichain.StepState, bool) {
	switch raw {
	case "preparing", "queued", "pending", "retrying", "custodian_reviewing":
		return multichain.StepPending, true
	case "confirmed":
		return multichain.StepConfirmed, true
	case "rejected", "reverted", "replaced", "abandoned", "failed":
		return multichain.StepFailed, true
	}
	return "", false
}

func decodeAction(w actionWire) (multichain.ActionStatus, error) {
	state, ok := actionState(w.Status)
	if !ok || w.ID == "" {
		return multichain.ActionStatus{}, errs.New(errs.CodeDecodeFailed, "privy.decodeAction",
			slog.String("status", w.Status))
	}
	out := multichain.ActionStatus{ID: multichain.ActionID(w.ID), State: state, Steps: []multichain.ActionStep{}}
	if w.DestinationAmount != "" {
		delivered, err := microsOfDecimal(w.DestinationAmount)
		if err != nil {
			return multichain.ActionStatus{}, errs.Wrap(err, errs.CodeDecodeFailed, "privy.decodeAction",
				slog.String("destination_amount", w.DestinationAmount))
		}
		out.Delivered = delivered
	}
	for _, s := range w.Steps {
		step, err := decodeStep(s)
		if err != nil {
			return multichain.ActionStatus{}, err
		}
		out.Steps = append(out.Steps, step)
	}
	return out, nil
}

func decodeStep(s stepWire) (multichain.ActionStep, error) {
	const op = "privy.decodeStep"
	state, ok := stepState(s.Status)
	if !ok {
		return multichain.ActionStep{}, errs.New(errs.CodeDecodeFailed, op, slog.String("status", s.Status))
	}
	var hash string
	switch s.Type {
	case "evm_transaction":
		hash = s.TransactionHash
	case "evm_user_operation":
		hash = s.BundleTransactionHash
	case "svm_transaction":
		hash = s.TransactionSignature
	case "external_transaction":
	default:
		return multichain.ActionStep{}, errs.New(errs.CodeDecodeFailed, op, slog.String("type", s.Type))
	}
	network, ok := networkOf(s.CAIP2)
	if !ok {
		return multichain.ActionStep{}, errs.New(errs.CodeDecodeFailed, op, slog.String("caip2", s.CAIP2))
	}
	return multichain.ActionStep{Network: network, TxHash: hash, State: state}, nil
}

func networkOf(caip2 string) (chain.Network, bool) {
	if caip2 == "" {
		return "", true
	}
	for _, n := range chain.Networks() {
		if n.CAIP2() == caip2 {
			return n, true
		}
	}
	return "", false
}
