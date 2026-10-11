package privy

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

var _ multichain.DepositForwarding = (*Client)(nil)

const depositForwardName = "monaco-evm-deposit-forward"

func depositForwardSources() []chain.Network {
	return []chain.Network{chain.NetworkBase, chain.NetworkArbitrum, chain.NetworkEthereum}
}

type automationWire struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type automationPage struct {
	Data       []automationWire `json:"data"`
	NextCursor string           `json:"next_cursor"`
}

type namedAsset struct {
	Asset string `json:"asset"`
	Chain string `json:"chain"`
}

type createAutomation struct {
	Name    string `json:"name"`
	OwnerID string `json:"owner_id"`
	Config  struct {
		Trigger struct {
			Type   string `json:"type"`
			Assets struct {
				Mode   string       `json:"mode"`
				Values []namedAsset `json:"values"`
			} `json:"assets"`
		} `json:"trigger"`
		Action struct {
			Type        string     `json:"type"`
			Destination namedAsset `json:"destination_chain_asset"`
		} `json:"action"`
	} `json:"config"`
}

func (c *Client) CreateAutomation(ctx context.Context) (multichain.AutomationID, error) {
	const op = "privy.CreateAutomation"
	if c.cfg.AuthorizationKeyID == "" {
		return "", errs.New(errs.CodeInternal, op, slog.String("reason", "authorization key id missing"))
	}
	found, err := c.automations(ctx, op)
	if err != nil {
		return "", err
	}
	for _, a := range found {
		if a.Name == depositForwardName && a.Status == "enabled" {
			return multichain.AutomationID(a.ID), nil
		}
	}
	var body createAutomation
	body.Name, body.OwnerID = depositForwardName, c.cfg.AuthorizationKeyID
	body.Config.Trigger.Type, body.Config.Trigger.Assets.Mode = "deposit", "include"
	for _, n := range depositForwardSources() {
		body.Config.Trigger.Assets.Values = append(body.Config.Trigger.Assets.Values,
			namedAsset{Asset: "usdc", Chain: n.PrivyChain()})
	}
	body.Config.Action.Type = "swap"
	body.Config.Action.Destination = namedAsset{Asset: "usdc", Chain: chain.NetworkSolana.PrivyChain()}
	var w automationWire
	in := call{
		op: op, method: http.MethodPost, path: "/v1/wallet_automations", body: body,
		headers: map[string]string{idempotencyHeader: "automation:" + depositForwardName},
	}
	if err := c.do(ctx, in, &w); err != nil {
		return "", err
	}
	if w.ID == "" {
		return "", errs.New(errs.CodeDecodeFailed, op)
	}
	return multichain.AutomationID(w.ID), nil
}

func (c *Client) automations(ctx context.Context, op string) ([]automationWire, error) {
	var out []automationWire
	cursor := ""
	for {
		q := url.Values{"limit": {"100"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var page automationPage
		in := call{op: op, method: http.MethodGet, path: "/v1/wallet_automations?" + q.Encode()}
		if err := c.do(ctx, in, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Data...)
		if page.NextCursor == "" || page.NextCursor == cursor {
			return out, nil
		}
		cursor = page.NextCursor
	}
}

type attachBody struct {
	AutomationIDs []string `json:"automation_ids"`
	Params        struct {
		DestinationAddress string `json:"destination_address"`
	} `json:"params"`
}

func (c *Client) Attach(ctx context.Context, evmWalletID string, solanaDest chain.SolanaAddress) error {
	const op = "privy.Attach"
	if c.authKey == nil {
		return errs.New(errs.CodeInternal, op, slog.String("reason", "authorization key missing"))
	}
	if _, err := chain.ParseAddress(string(solanaDest)); err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	id, err := c.CreateAutomation(ctx)
	if err != nil {
		return err
	}
	var body attachBody
	body.AutomationIDs, body.Params.DestinationAddress = []string{string(id)}, string(solanaDest)
	var w struct {
		Data []attachmentWire `json:"data"`
	}
	path := "/v1/wallets/" + url.PathEscape(evmWalletID) + "/automations/attach"
	key := "attach:" + evmWalletID + ":" + string(solanaDest)
	if err := c.do(ctx, c.signedPost(op, path, body, key), &w); err != nil {
		return err
	}
	return checkAttachment(op, w.Data, id, evmWalletID, solanaDest)
}

type attachmentWire struct {
	AutomationID string `json:"automation_id"`
	WalletID     string `json:"wallet_id"`
	Params       *struct {
		DestinationAddress string `json:"destination_address"`
	} `json:"params"`
}

func checkAttachment(
	op string, data []attachmentWire, id multichain.AutomationID, walletID string, dest chain.SolanaAddress,
) error {
	i := slices.IndexFunc(data, func(a attachmentWire) bool {
		return a.AutomationID == string(id) && a.WalletID == walletID && a.Params != nil
	})
	switch {
	case i < 0:
		return errs.New(errs.CodeDecodeFailed, op, slog.String("wallet_id", walletID))
	case data[i].Params.DestinationAddress != string(dest):
		return errs.New(errs.CodeWalletMismatch, op, slog.String("wallet_id", walletID),
			slog.String("attached_to", data[i].Params.DestinationAddress), slog.String("want", string(dest)))
	}
	return nil
}

type executionWire struct {
	ID       string  `json:"id"`
	CAIP2    string  `json:"trigger_caip2"`
	TxHash   string  `json:"trigger_tx_hash"`
	ActionID *string `json:"wallet_action_id"`
	Status   string  `json:"status"`
}

func executionState(raw string) (multichain.ActionState, bool) {
	switch raw {
	case "pending", "submitted":
		return multichain.ActionPending, true
	case "completed":
		return multichain.ActionSucceeded, true
	case "failed":
		return multichain.ActionFailed, true
	case "skipped":
		return multichain.ActionRejected, true
	}
	return "", false
}

func (c *Client) Executions(ctx context.Context, walletID, cursor string) (multichain.ExecutionPage, error) {
	const op = "privy.Executions"
	q := url.Values{"wallet_id": {walletID}, "limit": {"100"}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var page struct {
		Data       []executionWire `json:"data"`
		NextCursor string          `json:"next_cursor"`
	}
	in := call{op: op, method: http.MethodGet, path: "/v1/wallet_automations/executions?" + q.Encode()}
	if err := c.do(ctx, in, &page); err != nil {
		return multichain.ExecutionPage{}, err
	}
	out := multichain.ExecutionPage{Executions: []multichain.Execution{}, Next: page.NextCursor}
	for _, e := range page.Data {
		exec, err := c.execution(ctx, walletID, e)
		if err != nil {
			return multichain.ExecutionPage{}, errs.Wrap(err, errs.CodeOf(err), op, slog.String("execution_id", e.ID))
		}
		out.Executions = append(out.Executions, exec)
	}
	return out, nil
}

func (c *Client) execution(ctx context.Context, walletID string, e executionWire) (multichain.Execution, error) {
	const op = "privy.execution"
	state, ok := executionState(e.Status)
	network, known := networkOf(e.CAIP2)
	if !ok || !known || !network.IsEVM() || e.ID == "" {
		return multichain.Execution{}, errs.New(errs.CodeDecodeFailed, op,
			slog.String("status", e.Status), slog.String("caip2", e.CAIP2))
	}
	out := multichain.Execution{ID: e.ID, Network: network, SourceTx: e.TxHash, State: state}
	if e.ActionID == nil || *e.ActionID == "" {
		return out, nil
	}
	out.ActionID = multichain.ActionID(*e.ActionID)
	var w swapWire
	path := "/v1/wallets/" + url.PathEscape(walletID) + "/actions/" + url.PathEscape(*e.ActionID) + "?include=steps"
	if err := c.do(ctx, call{op: op, method: http.MethodGet, path: path}, &w); err != nil {
		return multichain.Execution{}, err
	}
	status, err := decodeAction(w.actionWire)
	if err != nil {
		return multichain.Execution{}, err
	}
	input, err := swapMicros(op, w.InputAmount)
	if err != nil {
		return multichain.Execution{}, err
	}
	output, err := swapMicros(op, w.OutputAmount)
	if err != nil {
		return multichain.Execution{}, err
	}
	if w.OutputAmount == nil {
		output, input = input, money.Micros{}
	}
	out.AmountMicros, out.InputMicros, out.Signature = output, input, solanaSignature(status.Steps)
	return out, nil
}

func swapMicros(op string, amount *string) (money.Micros, error) {
	if amount == nil {
		return money.Micros{}, nil
	}
	micros, err := strconv.ParseUint(*amount, 10, 64)
	if err != nil {
		return money.Micros{}, errs.New(errs.CodeDecodeFailed, op, slog.String("amount", *amount))
	}
	return money.MicrosFromUint64(micros), nil
}

func solanaSignature(steps []multichain.ActionStep) string {
	for _, s := range steps {
		if s.Network == chain.NetworkSolana && s.TxHash != "" {
			return s.TxHash
		}
	}
	return ""
}

type swapWire struct {
	actionWire
	InputAmount  *string `json:"input_amount"`
	OutputAmount *string `json:"output_amount"`
}

func (c *Client) Reindex(ctx context.Context, walletID string, network chain.Network) error {
	const op = "privy.Reindex"
	if !network.IsEVM() {
		return errs.New(errs.CodeInvalidNetwork, op, slog.String("network", string(network)))
	}
	body := map[string]string{
		"wallet_id": walletID, "caip2": network.CAIP2(), "asset_address": network.Cash().Address,
	}
	var w struct {
		Results []struct {
			Status string `json:"status"`
		} `json:"results"`
	}
	if err := c.do(
		ctx,
		call{op: op, method: http.MethodPost, path: "/v1/wallet_automations/reindex", body: body},
		&w,
	); err != nil {
		return err
	}
	for _, r := range w.Results {
		switch r.Status {
		case "submitted", "skipped_zero_balance", "skipped_no_match", "skipped_existing_execution":
		case "failed":
			return errs.New(errs.CodePrivyUnavailable, op, slog.String("wallet_id", walletID))
		default:
			return errs.New(errs.CodeDecodeFailed, op, slog.String("status", r.Status))
		}
	}
	return nil
}
