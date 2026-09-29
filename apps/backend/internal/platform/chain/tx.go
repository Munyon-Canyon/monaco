package chain

import (
	"crypto/ed25519"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Transaction struct {
	Signatures [][]byte
	Message    []byte
	Signers    []SolanaAddress
}

func DecodeTransaction(raw []byte) (Transaction, error) {
	const op = "chain.DecodeTransaction"
	n, rest, ok := compactU16(raw)
	if !ok || n == 0 || len(rest) < n*ed25519.SignatureSize {
		return Transaction{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "signatures"))
	}
	tx := Transaction{Message: rest[n*ed25519.SignatureSize:]}
	for i := range n {
		tx.Signatures = append(tx.Signatures, rest[i*ed25519.SignatureSize:(i+1)*ed25519.SignatureSize])
	}
	signers, err := signersOf(op, tx.Message, n)
	if err != nil {
		return Transaction{}, err
	}
	tx.Signers = signers
	return tx, nil
}

func signersOf(op string, msg []byte, n int) ([]SolanaAddress, error) {
	if len(msg) > 0 && msg[0]&0x80 != 0 {
		msg = msg[1:]
	}
	if len(msg) < 3 || int(msg[0]) != n {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "header"))
	}
	keys, rest, ok := compactU16(msg[3:])
	if !ok || keys < n || len(rest) < n*ed25519.PublicKeySize {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "account keys"))
	}
	signers := make([]SolanaAddress, 0, n)
	for i := range n {
		signers = append(signers, AddressOf(rest[i*ed25519.PublicKeySize:(i+1)*ed25519.PublicKeySize]))
	}
	return signers, nil
}

func (t Transaction) Encode() []byte {
	out := CompactU16(len(t.Signatures))
	for _, s := range t.Signatures {
		out = append(out, s...)
	}
	return append(out, t.Message...)
}

func (t Transaction) Sign(key ed25519.PrivateKey) error {
	signer := AddressOf(key.Public().(ed25519.PublicKey))
	for i, s := range t.Signers {
		if s == signer {
			t.Signatures[i] = ed25519.Sign(key, t.Message)
			return nil
		}
	}
	return errs.New(errs.CodeInvalidInput, "chain.Transaction.Sign", slog.String("signer", string(signer)))
}

func (t Transaction) Signed(i int) bool {
	pub, _ := t.Signers[i].Bytes()
	return ed25519.Verify(pub, t.Message, t.Signatures[i])
}

func CompactU16(n int) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n == 0 {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

func compactU16(raw []byte) (int, []byte, bool) {
	n := 0
	for i := 0; i < 3 && i < len(raw); i++ {
		n |= int(raw[i]&0x7f) << (7 * i)
		if raw[i]&0x80 == 0 {
			return n, raw[i+1:], true
		}
	}
	return 0, nil, false
}
