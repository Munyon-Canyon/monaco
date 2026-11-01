package chain

import (
	"encoding/hex"
	"log/slog"
	"strings"

	"golang.org/x/crypto/sha3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type EVMAddress [20]byte

func ParseEVMAddress(raw string) (EVMAddress, error) {
	var a EVMAddress
	digits, ok := strings.CutPrefix(raw, "0x")
	if !ok || len(digits) != 2*len(a) {
		return EVMAddress{}, invalidEVMAddress()
	}
	if _, err := hex.Decode(a[:], []byte(digits)); err != nil {
		return EVMAddress{}, invalidEVMAddress()
	}
	mixed := digits != strings.ToLower(digits) && digits != strings.ToUpper(digits)
	if mixed && a.Checksum() != raw {
		return EVMAddress{}, invalidEVMAddress()
	}
	return a, nil
}

func invalidEVMAddress() error {
	return errs.New(errs.CodeInvalidAddress, "chain.ParseEVMAddress", slog.String("reason", "not a valid address"))
}

func (a EVMAddress) String() string { return "0x" + hex.EncodeToString(a[:]) }

func (a EVMAddress) Checksum() string {
	lower := hex.EncodeToString(a[:])
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(lower))
	hash := h.Sum(nil)
	out := []byte(lower)
	for i, c := range out {
		nibble := hash[i/2] >> 4
		if i%2 == 1 {
			nibble = hash[i/2] & 0x0f
		}
		if c >= 'a' && nibble >= 8 {
			out[i] = c - 'a' + 'A'
		}
	}
	return "0x" + string(out)
}

func (a EVMAddress) MarshalText() ([]byte, error) { return []byte(a.String()), nil }

func (a *EVMAddress) UnmarshalText(text []byte) error {
	parsed, err := ParseEVMAddress(string(text))
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}
