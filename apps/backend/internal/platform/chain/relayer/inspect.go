package relayer

import (
	"crypto/ed25519"
	"log/slog"
	"math/bits"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	ComputeBudgetProgram chain.SolanaAddress = "ComputeBudget111111111111111111111111111111"
	JupiterV6Program     chain.SolanaAddress = "JUP6LkbZbjS1jKKwapdHNy74zcZ3tLUZoi5QNyVTaV4"
	JupiterZProgram      chain.SolanaAddress = "61DFfeTKM7trxYcPQCM78bJ794ddZprZpAwAnLiwTpYH"

	MaxPriorityFeeLamports   = 1_000_000
	MaxATACreations          = 2
	ATARentAllowanceLamports = 2_500_000
	MaxSOLOutLamports        = MaxPriorityFeeLamports + MaxATACreations*ATARentAllowanceLamports

	defaultComputeUnitLimit = 1_400_000
	blockhashSize           = 32
	microLamportsPerLamport = 1_000_000
)

const (
	budgetSetUnitLimit        = 2
	budgetSetUnitPrice        = 3
	budgetSetLoadedAccounts   = 4
	ataCreate                 = 0
	ataCreateIdempotent       = 1
	versionedMessagePrefixBit = 0x80
)

type swapInstruction struct {
	program chain.SolanaAddress
	data    []byte
}

type spend struct {
	unitLimit  uint64
	unitPrice  uint64
	ataCreates uint64
}

func isSwapProgram(program chain.SolanaAddress) bool {
	return program == JupiterV6Program || program == JupiterZProgram
}

func inspectSwap(tx chain.Transaction) error {
	const op = "relayer.inspectSwap"
	refuse := func(reason string, attrs ...slog.Attr) error {
		return errs.New(errs.CodeInvalidInput, op, append([]slog.Attr{slog.String("reason", reason)}, attrs...)...)
	}
	instructions, ok := instructionsOf(tx.Message)
	if !ok {
		return refuse("instructions")
	}
	out := spend{unitLimit: defaultComputeUnitLimit}
	for _, ix := range instructions {
		if reason := out.add(ix); reason != "" {
			return refuse(reason, slog.String("program", string(ix.program)))
		}
	}
	priority, ok := priorityLamports(out.unitPrice, out.unitLimit)
	if !ok || priority > MaxPriorityFeeLamports {
		return refuse("priority fee over the cap", slog.Uint64("cap_lamports", MaxPriorityFeeLamports))
	}
	if out.ataCreates > MaxATACreations || priority+out.ataCreates*ATARentAllowanceLamports > MaxSOLOutLamports {
		return refuse("sol out over the cap", slog.Uint64("cap_lamports", MaxSOLOutLamports))
	}
	return nil
}

func (s *spend) add(ix swapInstruction) string {
	switch {
	case ix.program == ComputeBudgetProgram:
		limit, price, ok := budgetOf(ix.data)
		if !ok {
			return "compute budget instruction"
		}
		if limit != nil {
			s.unitLimit = *limit
		}
		if price != nil {
			s.unitPrice = *price
		}
	case ix.program == chain.ATAProgram:
		if len(ix.data) > 1 || len(ix.data) == 1 && ix.data[0] > ataCreateIdempotent {
			return "associated token account instruction"
		}
		s.ataCreates++
	case !isSwapProgram(ix.program):
		return "program"
	}
	return ""
}

func priorityLamports(microLamportsPerUnit, units uint64) (uint64, bool) {
	hi, lo := bits.Mul64(microLamportsPerUnit, units)
	lo, carry := bits.Add64(lo, microLamportsPerLamport-1, 0)
	hi += carry
	if hi >= microLamportsPerLamport {
		return 0, false
	}
	lamports, _ := bits.Div64(hi, lo, microLamportsPerLamport)
	return lamports, true
}

func budgetOf(data []byte) (limit, price *uint64, ok bool) {
	if len(data) == 0 {
		return nil, nil, false
	}
	read := func(width int) (uint64, bool) {
		if len(data) != 1+width {
			return 0, false
		}
		var v uint64
		for i := width - 1; i >= 0; i-- {
			v = v<<8 | uint64(data[1+i])
		}
		return v, true
	}
	switch data[0] {
	case budgetSetUnitLimit:
		v, ok := read(4)
		return &v, nil, ok
	case budgetSetUnitPrice:
		v, ok := read(8)
		return nil, &v, ok
	case budgetSetLoadedAccounts:
		_, ok := read(4)
		return nil, nil, ok
	}
	return nil, nil, false
}

func instructionsOf(msg []byte) ([]swapInstruction, bool) {
	if len(msg) > 0 && msg[0]&versionedMessagePrefixBit != 0 {
		msg = msg[1:]
	}
	if len(msg) < 3 {
		return nil, false
	}
	keys, rest, ok := accountKeys(msg[3:])
	if !ok {
		return nil, false
	}
	n, rest, ok := shortVec(rest)
	if !ok {
		return nil, false
	}
	out := make([]swapInstruction, 0, n)
	for range n {
		var ix swapInstruction
		if ix, rest, ok = nextInstruction(keys, rest); !ok {
			return nil, false
		}
		out = append(out, ix)
	}
	return out, true
}

func accountKeys(b []byte) (keys []chain.SolanaAddress, rest []byte, ok bool) {
	count, rest, ok := shortVec(b)
	if !ok || len(rest) < count*ed25519.PublicKeySize+blockhashSize {
		return nil, nil, false
	}
	keys = make([]chain.SolanaAddress, count)
	for i := range keys {
		keys[i] = chain.AddressOf(rest[i*ed25519.PublicKeySize : (i+1)*ed25519.PublicKeySize])
	}
	return keys, rest[count*ed25519.PublicKeySize+blockhashSize:], true
}

func nextInstruction(keys []chain.SolanaAddress, b []byte) (ix swapInstruction, rest []byte, ok bool) {
	if len(b) < 1 || int(b[0]) >= len(keys) {
		return swapInstruction{}, nil, false
	}
	accounts, after, ok := shortVec(b[1:])
	if !ok || len(after) < accounts {
		return swapInstruction{}, nil, false
	}
	size, after, ok := shortVec(after[accounts:])
	if !ok || len(after) < size {
		return swapInstruction{}, nil, false
	}
	return swapInstruction{program: keys[b[0]], data: after[:size]}, after[size:], true
}

func shortVec(b []byte) (n int, rest []byte, ok bool) {
	for i := range 3 {
		if i >= len(b) {
			return 0, nil, false
		}
		n |= int(b[i]&0x7f) << (7 * i)
		if b[i]&0x80 == 0 {
			return n, b[i+1:], true
		}
	}
	return 0, nil, false
}
