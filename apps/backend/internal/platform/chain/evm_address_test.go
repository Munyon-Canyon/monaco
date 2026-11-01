package chain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func TestEVMAddressChecksumMatchesEIP55Vectors(t *testing.T) {
	t.Parallel()
	for _, want := range []string{
		"0x52908400098527886E0F7030069857D2E4169EE7",
		"0x8617E340B3D01FA5F11F306F4090FD50E238070D",
		"0xde709f2102306220921060314715629080e2fb77",
		"0x27b1fdb04752bbc536007a920d24acb045561c26",
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
		"0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
		"0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
	} {
		a, err := chain.ParseEVMAddress(want)
		if err != nil {
			t.Fatalf("ParseEVMAddress(%q) = %v", want, err)
		}
		if got := a.Checksum(); got != want {
			t.Errorf("Checksum() = %q, want %q", got, want)
		}
	}
}

func TestParseEVMAddress(t *testing.T) {
	t.Parallel()
	const lower = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"
	for _, tc := range []struct {
		raw string
		ok  bool
	}{
		{lower, true},
		{"0x5AAEB6053F3E94C9B9A09F33669435E7EF1BEAED", true},
		{"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", true},
		{"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD", false},
		{"5aaeb6053f3e94c9b9a09f33669435e7ef1beaed", false},
		{"0X5aaeb6053f3e94c9b9a09f33669435e7ef1beaed", false},
		{"0x5aaeb6053f3e94c9b9a09f33669435e7ef1bea", false},
		{"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaedff", false},
		{"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaeg", false},
		{"", false},
	} {
		a, err := chain.ParseEVMAddress(tc.raw)
		if !tc.ok {
			if errs.CodeOf(err) != errs.CodeInvalidAddress {
				t.Errorf("ParseEVMAddress(%q) error = %v, want invalid_address", tc.raw, err)
			}
			continue
		}
		if err != nil || a.String() != lower {
			t.Errorf("ParseEVMAddress(%q) = %q, %v, want %q", tc.raw, a, err, lower)
		}
	}
}

func TestEVMAddressJSONIsLowercase(t *testing.T) {
	t.Parallel()
	var got struct {
		A chain.EVMAddress `json:"A"`
	}
	if err := json.Unmarshal([]byte(`{"A":"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"}`), &got); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(got)
	if err != nil || string(out) != `{"A":"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"}` {
		t.Fatalf("Marshal = %s, %v", out, err)
	}
	if err := json.Unmarshal([]byte(`{"A":"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD"}`), &got); err == nil {
		t.Fatal("Unmarshal accepted a bad checksum")
	}
}

func TestRejectedAddressesDoNotCarryTheTypedValue(t *testing.T) {
	t.Parallel()
	const typed = "0xNotAnAddressTypedByAUser0000000000000000"
	_, evmErr := chain.ParseEVMAddress(typed)
	_, solErr := chain.ParseAddress(typed)
	for name, err := range map[string]error{"evm": evmErr, "solana": solErr} {
		if errs.CodeOf(err) != errs.CodeInvalidAddress {
			t.Fatalf("%s err = %v, want invalid_address", name, err)
		}
		for _, attr := range errs.Detail(err) {
			if strings.Contains(attr.Value.String(), typed) || attr.Key == "address" {
				t.Fatalf("%s detail %v carries the typed address", name, attr)
			}
		}
	}
}
