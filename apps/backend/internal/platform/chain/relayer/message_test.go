package relayer_test

import (
	"bytes"
	"crypto/ed25519"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	headerSize    = 3
	blockhashSize = 32
	versionPrefix = 0x80
)

type rawIx struct {
	program  int
	accounts []int
	data     []byte
}

type txMessage struct {
	signatures   int
	versioned    bool
	header       [headerSize]byte
	keys         []chain.SolanaAddress
	blockhash    []byte
	instructions []rawIx
	tail         []byte
}

type cursor struct {
	t *testing.T
	b []byte
}

func (c *cursor) take(n int) []byte {
	c.t.Helper()
	if len(c.b) < n {
		c.t.Fatalf("message ends %d bytes short", n-len(c.b))
	}
	out := c.b[:n]
	c.b = c.b[n:]
	return out
}

func (c *cursor) length() int {
	c.t.Helper()
	n, shift := 0, 0
	for {
		b := c.take(1)[0]
		n |= int(b&0x7f) << shift
		if b&0x80 == 0 {
			return n
		}
		shift += 7
	}
}

func decodeMessage(t *testing.T, raw []byte) txMessage {
	t.Helper()
	tx, err := chain.DecodeTransaction(raw)
	if err != nil {
		t.Fatal(err)
	}
	m := txMessage{signatures: len(tx.Signatures)}
	c := &cursor{t: t, b: tx.Message}
	if c.b[0]&versionPrefix != 0 {
		m.versioned = true
		c.take(1)
	}
	copy(m.header[:], c.take(headerSize))
	for range c.length() {
		m.keys = append(m.keys, chain.AddressOf(c.take(ed25519.PublicKeySize)))
	}
	m.blockhash = bytes.Clone(c.take(blockhashSize))
	for range c.length() {
		ix := rawIx{program: int(c.take(1)[0])}
		for _, a := range c.take(c.length()) {
			ix.accounts = append(ix.accounts, int(a))
		}
		ix.data = bytes.Clone(c.take(c.length()))
		m.instructions = append(m.instructions, ix)
	}
	m.tail = bytes.Clone(c.b)
	return m
}

func (m txMessage) encode() []byte {
	var msg []byte
	if m.versioned {
		msg = append(msg, versionPrefix)
	}
	msg = append(msg, m.header[:]...)
	msg = append(msg, chain.CompactU16(len(m.keys))...)
	for _, k := range m.keys {
		raw, _ := k.Bytes()
		msg = append(msg, raw...)
	}
	msg = append(msg, m.blockhash...)
	msg = append(msg, chain.CompactU16(len(m.instructions))...)
	for _, ix := range m.instructions {
		msg = append(msg, byteOf(ix.program))
		msg = append(msg, chain.CompactU16(len(ix.accounts))...)
		for _, a := range ix.accounts {
			msg = append(msg, byteOf(a))
		}
		msg = append(msg, chain.CompactU16(len(ix.data))...)
		msg = append(msg, ix.data...)
	}
	msg = append(msg, m.tail...)
	return append(append(chain.CompactU16(m.signatures), make([]byte, m.signatures*ed25519.SignatureSize)...), msg...)
}

func (m txMessage) clone() txMessage {
	out := m
	out.keys = slices.Clone(m.keys)
	out.instructions = make([]rawIx, len(m.instructions))
	for i, ix := range m.instructions {
		out.instructions[i] = rawIx{
			program:  ix.program,
			accounts: slices.Clone(ix.accounts),
			data:     bytes.Clone(ix.data),
		}
	}
	return out
}

func (m *txMessage) keyIndex(addr chain.SolanaAddress) int {
	if i := slices.Index(m.keys, addr); i >= 0 {
		return i
	}
	staticKeys := len(m.keys)
	for _, ix := range m.instructions {
		for j, a := range ix.accounts {
			if a >= staticKeys {
				ix.accounts[j] = a + 1
			}
		}
	}
	m.keys = append(m.keys, addr)
	m.header[2]++
	return staticKeys
}

func (m txMessage) with(extra ...rawIx) txMessage {
	out := m.clone()
	out.instructions = append(out.instructions, extra...)
	return out
}

func (m txMessage) mapData(program chain.SolanaAddress, change func([]byte) []byte) txMessage {
	out := m.clone()
	for i, ix := range out.instructions {
		if out.keys[ix.program] == program {
			out.instructions[i].data = change(ix.data)
		}
	}
	return out
}

func byteOf(n int) byte { return byte(n & 0xff) }
