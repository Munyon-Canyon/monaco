package adapters

import (
	"encoding/hex"
	"log/slog"
	"math/big"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

const (
	executeSelector    = "3593564c"
	executeNoDeadline  = "24856bc3"
	selectorSize       = 4
	addressLen         = 20
	v3HopLen           = 23
	commandTypeMask    = 0x3f
	allowRevertFlag    = 0x80
	cmdV3SwapExactIn   = 0x00
	cmdPermit2Transfer = 0x02
	cmdSweep           = 0x04
	cmdV2SwapExactIn   = 0x08
	cmdBalanceCheck    = 0x0e
	cmdV4Swap          = 0x10
	v4SwapExactInOne   = 0x06
	v4SwapExactIn      = 0x07
	v4SettleAll        = 0x0c
	v4TakeAll          = 0x0f
)

const (
	calldataOp  = "trading.EVMVenue.swapCalldata"
	maxArrayLen = 64
)

func routerHolds() chain.EVMAddress { return chain.EVMAddress{addressLen - 1: 2} }

func msgSender() chain.EVMAddress { return chain.EVMAddress{addressLen - 1: 1} }

func errCalldata(reason string, attrs ...slog.Attr) error {
	return errs.New(errs.CodeDecodeFailed, calldataOp, append(attrs, slog.String("reason", reason))...)
}

type frame []byte

func (f frame) word(i int) ([]byte, bool) {
	if i < 0 || (i+1)*abiWord > len(f) {
		return nil, false
	}
	return f[i*abiWord : (i+1)*abiWord], true
}

func (f frame) num(i int) (*big.Int, bool) {
	w, ok := f.word(i)
	return new(big.Int).SetBytes(w), ok
}

func (f frame) addr(i int) (chain.EVMAddress, bool) {
	var a chain.EVMAddress
	w, ok := f.word(i)
	if !ok {
		return a, false
	}
	copy(a[:], w[addressPadding:])
	for _, b := range w[:addressPadding] {
		if b != 0 {
			return a, false
		}
	}
	return a, true
}

func (f frame) flag(i int) (bool, bool) {
	n, ok := f.num(i)
	return n.Sign() != 0, ok && n.BitLen() <= 1
}

func (f frame) at(i int) (frame, bool) {
	n, ok := f.num(i)
	if !ok || !n.IsInt64() || n.Int64() > int64(len(f)) {
		return nil, false
	}
	return f[n.Int64():], true
}

func (f frame) data() ([]byte, bool) {
	n, ok := f.num(0)
	if !ok || !n.IsInt64() || n.Int64() > int64(len(f)-abiWord) {
		return nil, false
	}
	return f[abiWord : abiWord+n.Int64()], true
}

func (f frame) bytesAt(i int) ([]byte, bool) {
	sub, ok := f.at(i)
	if !ok {
		return nil, false
	}
	return sub.data()
}

func (f frame) items(i int) ([]frame, bool) {
	sub, ok := f.at(i)
	if !ok {
		return nil, false
	}
	n, ok := sub.num(0)
	if !ok || !n.IsInt64() || n.Int64() > int64(maxArrayLen) {
		return nil, false
	}
	area := sub[abiWord:]
	out := make([]frame, 0, n.Int64())
	for j := range int(n.Int64()) {
		item, ok := area.at(j)
		if !ok {
			return nil, false
		}
		out = append(out, item)
	}
	return out, true
}

type swapLedger struct {
	spec      app.EVMSwapSpec
	pulled    *big.Int
	delivered *big.Int
	routed    bool
	swept     bool
	swaps     int
}

func checkSwapCalldata(spec app.EVMSwapSpec, tx mc.EVMTransaction, floor uint64) error {
	if len(tx.Data) < selectorSize {
		return errCalldata("calldata shorter than a selector")
	}
	selector := hex.EncodeToString(tx.Data[:selectorSize])
	if selector != executeSelector && selector != executeNoDeadline {
		return errCalldata("not a Universal Router execute call", slog.String("selector", selector))
	}
	args := frame(tx.Data[selectorSize:])
	commands, ok := args.bytesAt(0)
	inputs, okInputs := args.items(1)
	if !ok || !okInputs || len(commands) != len(inputs) || len(commands) == 0 {
		return errCalldata("commands and inputs do not match")
	}
	ledger := &swapLedger{spec: spec, pulled: new(big.Int), delivered: new(big.Int)}
	for i, c := range commands {
		input, ok := inputs[i].data()
		if !ok {
			return errCalldata("command input", slog.Int("index", i))
		}
		if err := ledger.command(c, input); err != nil {
			return err
		}
	}
	return ledger.settle(floor)
}

func (l *swapLedger) settle(floor uint64) error {
	switch {
	case l.swaps == 0:
		return errCalldata("no swap command")
	case l.pulled.Cmp(new(big.Int).SetUint64(l.spec.AmountIn)) > 0:
		return errCalldata("calldata pulls more than the approved input", slog.String("pulled", l.pulled.String()))
	case l.routed && !l.swept:
		return errCalldata("output left in the router")
	case l.delivered.Cmp(new(big.Int).SetUint64(floor)) < 0:
		return errCalldata("minimum output below the approved floor",
			slog.String("delivered", l.delivered.String()), slog.Uint64("floor", floor))
	}
	return nil
}

func (l *swapLedger) command(c byte, input []byte) error {
	if c&allowRevertFlag != 0 {
		return errCalldata("command may revert silently", slog.Int("command", int(c)))
	}
	args := frame(input)
	switch c & commandTypeMask {
	case cmdV3SwapExactIn:
		return l.v3(args)
	case cmdV2SwapExactIn:
		return l.v2(args)
	case cmdV4Swap:
		return l.v4(args)
	case cmdPermit2Transfer:
		return l.permit2Transfer(args)
	case cmdSweep:
		return l.sweep(args)
	case cmdBalanceCheck:
		return nil
	}
	return errCalldata("command outside the swap allowlist", slog.Int("command", int(c)))
}

func (l *swapLedger) v3(args frame) error {
	path, ok := args.bytesAt(3)
	if !ok || len(path) < addressLen+v3HopLen || (len(path)-addressLen)%v3HopLen != 0 {
		return errCalldata("v3 path")
	}
	first := path[:addressLen]
	last := path[len(path)-addressLen:]
	return l.exactIn(args, first, last)
}

func (l *swapLedger) v2(args frame) error {
	sub, ok := args.at(3)
	n, okN := sub.num(0)
	if !ok || !okN || !n.IsInt64() || n.Int64() < 2 || n.Int64() > int64(maxArrayLen) {
		return errCalldata("v2 path")
	}
	area := sub[abiWord:]
	first, okFirst := area.addr(0)
	last, okLast := area.addr(int(n.Int64()) - 1)
	if !okFirst || !okLast {
		return errCalldata("v2 path")
	}
	return l.exactIn(args, first[:], last[:])
}

func (l *swapLedger) exactIn(args frame, first, last []byte) error {
	recipient, okRecipient := args.addr(0)
	amountIn, okIn := args.num(1)
	minOut, okOut := args.num(2)
	payerIsUser, okPayer := args.flag(4)
	switch {
	case !okRecipient || !okIn || !okOut || !okPayer:
		return errCalldata("swap arguments")
	case !slices.Equal(first, l.spec.TokenIn[:]) || !slices.Equal(last, l.spec.TokenOut[:]):
		return errCalldata("swap path does not run from the approved input to the approved output")
	case amountIn.Cmp(new(big.Int).SetUint64(l.spec.AmountIn)) > 0:
		return errCalldata("swap input above the approved amount", slog.String("amount_in", amountIn.String()))
	}
	l.swaps++
	if payerIsUser {
		l.pulled.Add(l.pulled, amountIn)
	}
	return l.output(recipient, minOut)
}

func (l *swapLedger) output(recipient chain.EVMAddress, minOut *big.Int) error {
	switch recipient {
	case l.spec.Owner, msgSender():
		l.delivered.Add(l.delivered, minOut)
	case routerHolds():
		l.routed = true
	default:
		return errCalldata("swap pays a third party", slog.String("recipient", recipient.String()))
	}
	return nil
}

func (l *swapLedger) permit2Transfer(args frame) error {
	token, okToken := args.addr(0)
	recipient, okRecipient := args.addr(1)
	amount, okAmount := args.num(2)
	if !okToken || !okRecipient || !okAmount || token != l.spec.TokenIn || recipient != routerHolds() {
		return errCalldata("permit2 transfer is not the approved input into the router")
	}
	l.pulled.Add(l.pulled, amount)
	return nil
}

func (l *swapLedger) sweep(args frame) error {
	token, okToken := args.addr(0)
	recipient, okRecipient := args.addr(1)
	minimum, okMin := args.num(2)
	if !okToken || !okRecipient || !okMin || (recipient != l.spec.Owner && recipient != msgSender()) {
		return errCalldata("sweep pays a third party")
	}
	switch token {
	case l.spec.TokenOut:
		l.delivered.Add(l.delivered, minimum)
		l.swept = true
	case l.spec.TokenIn:
	default:
		return errCalldata("sweep of a token outside the trade", slog.String("token", token.String()))
	}
	return nil
}

func (l *swapLedger) v4(args frame) error {
	actions, okActions := args.bytesAt(0)
	params, okParams := args.items(1)
	if !okActions || !okParams || len(actions) != len(params) {
		return errCalldata("v4 actions")
	}
	for i, a := range actions {
		raw, ok := params[i].data()
		if !ok {
			return errCalldata("v4 params", slog.Int("index", i))
		}
		if err := l.v4Action(a, frame(raw)); err != nil {
			return err
		}
	}
	return nil
}

func (l *swapLedger) v4Action(action byte, params frame) error {
	switch action {
	case v4SwapExactInOne:
		return l.v4Single(params)
	case v4SwapExactIn:
		return l.v4Path(params)
	case v4SettleAll:
		currency, okCurrency := params.addr(0)
		maximum, okMax := params.num(1)
		if !okCurrency || !okMax || currency != l.spec.TokenIn {
			return errCalldata("v4 settle is not the approved input")
		}
		l.pulled.Add(l.pulled, maximum)
		return nil
	case v4TakeAll:
		currency, okCurrency := params.addr(0)
		minimum, okMin := params.num(1)
		if !okCurrency || !okMin || currency != l.spec.TokenOut {
			return errCalldata("v4 take is not the approved output")
		}
		l.delivered.Add(l.delivered, minimum)
		l.swept = true
		return nil
	}
	return errCalldata("v4 action outside the swap allowlist", slog.Int("action", int(action)))
}

func (l *swapLedger) v4Single(params frame) error {
	s, ok := params.at(0)
	zero, okZero := s.flag(5)
	amountIn, okIn := s.num(6)
	currency0, ok0 := s.addr(0)
	currency1, ok1 := s.addr(1)
	if !ok || !okZero || !okIn || !ok0 || !ok1 {
		return errCalldata("v4 single swap")
	}
	if zero {
		return l.v4Swap(currency0, currency1, amountIn)
	}
	return l.v4Swap(currency1, currency0, amountIn)
}

func (l *swapLedger) v4Path(params frame) error {
	s, ok := params.at(0)
	currencyIn, okCurrency := s.addr(0)
	amountIn, okIn := s.num(2)
	hops, okHops := s.items(1)
	if !ok || !okCurrency || !okIn || !okHops || len(hops) == 0 {
		return errCalldata("v4 path swap")
	}
	currencyOut, okOut := hops[len(hops)-1].addr(0)
	if !okOut {
		return errCalldata("v4 path swap")
	}
	return l.v4Swap(currencyIn, currencyOut, amountIn)
}

func (l *swapLedger) v4Swap(in, out chain.EVMAddress, amountIn *big.Int) error {
	if in != l.spec.TokenIn || out != l.spec.TokenOut ||
		amountIn.Cmp(new(big.Int).SetUint64(l.spec.AmountIn)) > 0 {
		return errCalldata("v4 swap does not run from the approved input to the approved output within the amount")
	}
	l.swaps++
	return nil
}
