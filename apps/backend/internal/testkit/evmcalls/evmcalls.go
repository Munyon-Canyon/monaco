package evmcalls

import (
	"encoding/hex"
	"math/big"
	"slices"
	"strings"
)

const wordSize = 32

func Hex(b []byte) string { return "0x" + hex.EncodeToString(b) }

func Addr(a string) []byte {
	raw, err := hex.DecodeString(strings.TrimPrefix(a, "0x"))
	if err != nil || len(raw) != 20 {
		panic("evmcalls: bad address " + a)
	}
	return raw
}

func AddrWord(a string) []byte { return slices.Concat(make([]byte, 12), Addr(a)) }

func Uint(v uint64) []byte { return new(big.Int).SetUint64(v).FillBytes(make([]byte, wordSize)) }

func Big(v *big.Int) []byte { return v.FillBytes(make([]byte, wordSize)) }

func Bool(v bool) []byte {
	if v {
		return Uint(1)
	}
	return Uint(0)
}

func Bytes(b []byte) []byte {
	pad := (wordSize - len(b)%wordSize) % wordSize
	return slices.Concat(Uint(uint64(len(b))), b, make([]byte, pad))
}

func Heads(heads [][]byte, tails [][]byte) []byte {
	offset := wordSize * len(heads)
	var head, tail []byte
	for i, h := range heads {
		if h != nil {
			head = append(head, h...)
			continue
		}
		head = append(head, Uint(uint64(offset+len(tail)))...)
		tail = append(tail, tails[i]...)
	}
	return append(head, tail...)
}

func BytesArray(items ...[]byte) []byte {
	offsets := make([]byte, 0, len(items)*wordSize)
	body := make([]byte, 0, len(items)*2*wordSize)
	base := len(items) * wordSize
	for _, it := range items {
		offsets = append(offsets, Uint(uint64(base+len(body)))...)
		body = append(body, Bytes(it)...)
	}
	return slices.Concat(Uint(uint64(len(items))), offsets, body)
}

func Execute(commands []byte, inputs ...[]byte) []byte {
	args := Heads([][]byte{nil, nil, Uint(1 << 40)}, [][]byte{Bytes(commands), BytesArray(inputs...), nil})
	return slices.Concat(selector("3593564c"), args)
}

func ExecuteNoDeadline(commands []byte, inputs ...[]byte) []byte {
	args := Heads([][]byte{nil, nil}, [][]byte{Bytes(commands), BytesArray(inputs...)})
	return slices.Concat(selector("24856bc3"), args)
}

func selector(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
}

func V3Path(tokens ...string) []byte {
	var out []byte
	for i, t := range tokens {
		if i > 0 {
			out = append(out, 0x00, 0x0b, 0xb8)
		}
		out = append(out, Addr(t)...)
	}
	return out
}

func V3In(recipient string, amountIn, minOut uint64, path []byte, payerIsUser bool) []byte {
	return Heads(
		[][]byte{AddrWord(recipient), Uint(amountIn), Uint(minOut), nil, Bool(payerIsUser)},
		[][]byte{nil, nil, nil, Bytes(path), nil},
	)
}

func V2In(recipient string, amountIn, minOut uint64, path []string, payerIsUser bool) []byte {
	words := Uint(uint64(len(path)))
	for _, p := range path {
		words = append(words, AddrWord(p)...)
	}
	return Heads(
		[][]byte{AddrWord(recipient), Uint(amountIn), Uint(minOut), nil, Bool(payerIsUser)},
		[][]byte{nil, nil, nil, words, nil},
	)
}

func Permit2Transfer(token, recipient string, amount uint64) []byte {
	return slices.Concat(AddrWord(token), AddrWord(recipient), Uint(amount))
}

func Sweep(token, recipient string, minimum uint64) []byte {
	return slices.Concat(AddrWord(token), AddrWord(recipient), Uint(minimum))
}

func V4(actions []byte, params ...[]byte) []byte {
	return Heads([][]byte{nil, nil}, [][]byte{Bytes(actions), BytesArray(params...)})
}

func V4Single(currency0, currency1 string, zeroForOne bool, amountIn, minOut uint64) []byte {
	inner := slices.Concat(
		AddrWord(
			currency0,
		),
		AddrWord(currency1),
		Uint(3000),
		Uint(60),
		AddrWord("0x0000000000000000000000000000000000000000"),
		Bool(zeroForOne),
		Uint(amountIn),
		Uint(minOut),
		Uint(9*wordSize),
		Bytes(nil),
	)
	return slices.Concat(Uint(wordSize), inner)
}

func V4Path(currencyIn string, hops []string, amountIn, minOut uint64) []byte {
	elements := make([]byte, 0, len(hops)*6*wordSize)
	offsets := make([]byte, 0, len(hops)*wordSize)
	for _, h := range hops {
		offsets = append(offsets, Uint(uint64(len(hops)*wordSize+len(elements)))...)
		elements = append(elements, slices.Concat(
			AddrWord(
				h,
			),
			Uint(3000),
			Uint(60),
			AddrWord("0x0000000000000000000000000000000000000000"),
			Uint(5*wordSize),
			Bytes(nil),
		)...)
	}
	path := slices.Concat(Uint(uint64(len(hops))), offsets, elements)
	inner := slices.Concat(AddrWord(currencyIn), Uint(4*wordSize), Uint(amountIn), Uint(minOut), path)
	return slices.Concat(Uint(wordSize), inner)
}

func V4Pair(currency string, amount uint64) []byte {
	return slices.Concat(AddrWord(currency), Uint(amount))
}

func Honest(owner, tokenIn, tokenOut string, amountIn, minOut uint64) []byte {
	return Execute([]byte{0x00}, V3In(owner, amountIn, minOut, V3Path(tokenIn, tokenOut), true))
}
