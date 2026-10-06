package observability

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const (
	goldenPath   = "testdata/redaction.golden"
	sampleJWT    = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyIn0.c2lnbmF0dXJl"
	samplePubkey = "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU"

	sampleFundLinkValue = "q2J9cZQxv0mYb5r8yS3dTt1uVw7xY9zA0bC2dE4fG6h"
)

func sampleBase58Secret() string { return strings.Repeat("5Kd3", 22) }

func sampleByteArray() string { return "[" + strings.Repeat("12, ", 63) + "255]" }

type contact struct {
	Name   string
	Email  string
	Token  string `json:"access_token"`
	Nested *contact
	hidden string
}

func opaque(s string) string { return "opaque-" + s }

func selfLoop() *contact {
	c := &contact{Name: "loop"}
	c.Nested = c
	return c
}

type jwtHolder struct{}

func (jwtHolder) LogValue() slog.Value { return slog.StringValue(sampleJWT) }

type redactionCase struct {
	name  string
	build func(h slog.Handler) slog.Handler
	attrs []slog.Attr
}

func redactionCases() []redactionCase {
	keep := func(h slog.Handler) slog.Handler { return h }
	var cases []redactionCase
	for _, key := range []string{
		"phone", "email", "token", "key", "seed", "signature", "secret", "authorization", "EMAIL", "Authorization",
	} {
		cases = append(cases, redactionCase{"key " + key, keep, []slog.Attr{slog.String(key, "plain-value")}})
	}
	return append(
		cases,
		redactionCase{"identity keys", keep, []slog.Attr{
			slog.String("email", "identity@x.io"), slog.String("phone_e164", "+15550142"),
			slog.Any("phone_hash", []byte("hash-of-the-phone")), slog.String("x_user_id", "x-id-9001"),
			slog.String("x_username", "xhandle_leak"), slog.String("xUsername", "camel_xhandle"),
		}},
		redactionCase{"kept: x words that are not an x account", keep, []slog.Attr{
			slog.String("tx_user_id", "u1"), slog.String("username", "not_an_x_handle"),
		}},
		redactionCase{"key on int value", keep, []slog.Attr{slog.Int("seed", 42)}},
		redactionCase{"key on group value", keep, []slog.Attr{slog.Group("secret", slog.String("a", "b"))}},
		redactionCase{"nested group key", keep, []slog.Attr{
			slog.Group("user", slog.String("id", "u1"), slog.Group("contact", slog.String("phone", "+15550100"))),
		}},
		redactionCase{"base58 secret key value", keep, []slog.Attr{slog.String("relayer", sampleBase58Secret())}},
		redactionCase{"byte array secret key value", keep, []slog.Attr{slog.String("keypair", sampleByteArray())}},
		redactionCase{"jwt value", keep, []slog.Attr{slog.String("header", "Bearer "+sampleJWT)}},
		redactionCase{"jwt inside error", keep, []slog.Attr{slog.Any("err", errors.New("privy said "+sampleJWT))}},
		redactionCase{"jwt from log valuer", keep, []slog.Attr{slog.Any("session", jwtHolder{})}},
		redactionCase{"jwt in nested group", keep, []slog.Attr{slog.Group("req", slog.String("cookie", sampleJWT))}},
		redactionCase{"with attrs", func(h slog.Handler) slog.Handler {
			return h.WithAttrs([]slog.Attr{slog.String("token", "t"), slog.String("cabal_id", "c1")})
		}, []slog.Attr{slog.String("email", "a@b.c")}},
		redactionCase{"with group then attrs", func(h slog.Handler) slog.Handler {
			return h.WithGroup("privy").WithAttrs([]slog.Attr{slog.String("authorization", "Basic x")})
		}, []slog.Attr{slog.String("key", "k")}},
		redactionCase{"kept: wallet address", keep, []slog.Attr{slog.String("wallet", samplePubkey)}},
		redactionCase{"kept: ordinary values", keep, []slog.Attr{
			slog.String("cabal_id", "c1"), slog.Int64("have", 4_000_000), slog.Bool("ok", true),
			slog.Any("err", errors.New("jupiter unavailable")),
		}},
		redactionCase{"key suffix and token names", keep, []slog.Attr{
			slog.String("user_email", "u@x.io"), slog.String("privy_token", "opaque-privy"),
			slog.String("api_key", "opaque-api"), slog.String("phone_number", "+15550199"),
			slog.String("userEmail", "camel@x.io"), slog.String("API_KEY", "opaque-upper"),
			slog.String("tx_signature", "sig-1"), slog.String("__Secret__", "opaque-underscored"),
		}},
		redactionCase{"self referencing struct stops at depth", keep, []slog.Attr{slog.Any("loop", selfLoop())}},
		redactionCase{"kept: key word inside another word", keep, []slog.Attr{
			slog.String("monkey", "m"), slog.String("keyboard", "k"), slog.String("cabal_id", "c1"),
		}},
		redactionCase{"kept: idempotency key is evidence", keep, []slog.Attr{
			slog.String("idempotency_key", "idem-1"), slog.String("Idempotency-Key", "idem-2"),
			slog.String("api_key", "opaque-beside-idem"),
		}},
		redactionCase{"slice of structs", keep, []slog.Attr{slog.Any("users", []struct{ Email, Name string }{
			{Email: "slice@example.com", Name: "ada"}, {Email: "slice2@example.com", Name: "bob"},
		})}},
		redactionCase{"slice of maps", keep, []slog.Attr{slog.Any("creds", []map[string]string{
			{"token": opaque("slice-map"), "cluster": "mainnet"},
		})}},
		redactionCase{
			"array of strings with a jwt",
			keep,
			[]slog.Attr{slog.Any("headers", [2]string{"ok", sampleJWT})},
		},
		redactionCase{"onramp link token", keep, []slog.Attr{
			slog.String("token", sampleFundLinkValue), slog.String("onramp_token", sampleFundLinkValue),
			slog.String("fund_url", "https://monacolabs.xyz/fund?s="+sampleFundLinkValue),
			slog.Group("body", slog.String("token", sampleFundLinkValue)),
		}},
		redactionCase{"opaque authorization scheme", keep, []slog.Attr{slog.String("header", "Bearer opaque-bearer")}},
		redactionCase{"sensitive query params", keep, []slog.Attr{
			slog.String("url", "https://rpc.example/v1?api_key=opaque-query&cluster=mainnet&Token=opaque-query2"),
		}},
		redactionCase{"struct with sensitive fields", keep, []slog.Attr{slog.Any("user", contact{
			Name: "ada", Email: "struct@x.io", Token: opaque("struct"),
			Nested: &contact{Name: "bob", Email: "nested@x.io"}, hidden: "h",
		})}},
		redactionCase{
			"pointer to struct",
			keep,
			[]slog.Attr{slog.Any("user", &contact{Name: "cy", Email: "ptr@x.io"})},
		},
		redactionCase{"map with sensitive keys", keep, []slog.Attr{
			slog.Any("creds", map[string]any{"token": opaque("map"), "cluster": "mainnet", "inner": map[string]string{
				"api_key": opaque("inner"),
			}}),
		}},
		redactionCase{"64 byte key as bytes", keep, []slog.Attr{
			slog.Any("relayer", ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))),
			slog.Any("raw", [64]byte{1, 2, 3}),
			slog.Any("short", []byte("not a key")),
		}},
		redactionCase{"64 byte key as base64", keep, []slog.Attr{
			slog.String("blob", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 64))),
		}},
		redactionCase{"kept: nil pointer and slice", keep, []slog.Attr{
			slog.Any("user", (*contact)(nil)), slog.Any("ids", []string{"a", "b"}), slog.Any("none", []string(nil)),
		}},
	)
}

func TestRedaction_matchesGolden(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for _, c := range redactionCases() {
		h := c.build(NewLogger(config.Config{Env: config.EnvProduction}, &buf).Handler())
		r := slog.NewRecord(at, slog.LevelInfo, "redaction.case", 0)
		r.AddAttrs(slog.String("case", c.name))
		r.AddAttrs(c.attrs...)
		if err := h.Handle(t.Context(), r); err != nil {
			t.Fatal(err)
		}
	}
	if *update {
		if err := os.WriteFile(goldenPath, buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != string(want) {
		t.Errorf("redaction output differs from %s (rerun with -update to accept):\n%s", goldenPath, got)
	}
	for _, secret := range []string{
		sampleJWT, sampleBase58Secret(), sampleByteArray(), "a@b.c", "+15550100",
		"u@x.io", "opaque-", "+15550199", "camel@x.io", "sig-1", "slice@example.com", "slice2@example.com",
		"struct@x.io", "nested@x.io", "ptr@x.io", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 64)),
		"AAAAAAAAAAAA", "[1,2,3", "identity@x.io", "+15550142", "hash-of-the-phone", "aGFzaC1vZi10aGUtcGhvbmU",
		"x-id-9001", "xhandle_leak", "camel_xhandle",
	} {
		if strings.Contains(buf.String(), secret) {
			t.Errorf("output leaks %q", secret)
		}
	}
	for _, evidence := range []string{`"idempotency_key":"idem-1"`, `"Idempotency-Key":"idem-2"`, `"api_key":"***"`} {
		if !strings.Contains(buf.String(), evidence) {
			t.Errorf("output lacks %s", evidence)
		}
	}
}

func TestRedaction_plantedEmailPrintsMask(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	NewLogger(config.Config{}, &buf).InfoContext(t.Context(), "user.signed_in", slog.String("email", "a@b.c"))
	if got := buf.String(); !strings.Contains(got, `"email":"***"`) || strings.Contains(got, "a@b.c") {
		t.Fatalf("line = %s, want email masked as ***", got)
	}
}

func TestRedaction_aHashesKeyIsMasked(t *testing.T) {
	t.Parallel()
	const hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var buf bytes.Buffer
	NewLogger(config.Config{}, &buf).InfoContext(t.Context(), "social.contacts_matched", slog.String("hashes", hash))
	if got := buf.String(); !strings.Contains(got, `"hashes":"***"`) || strings.Contains(got, hash) {
		t.Fatalf("line = %s, want hashes masked as ***", got)
	}
}

func groupChain(depth int) slog.Attr {
	a := slog.String("leaf", "deep-value")
	for i := depth; i >= 0; i-- {
		a = slog.Group("g"+strings.Repeat("x", i), a)
	}
	return a
}

func TestRedact_masksAGroupOnlyBelowTheEighthLevel(t *testing.T) {
	t.Parallel()
	got := redact(groupChain(11))
	for level := range 8 {
		if got.Value.Kind() != slog.KindGroup || len(got.Value.Group()) != 1 {
			t.Fatalf("level %d = %v, want a group with one attribute", level, got)
		}
		got = got.Value.Group()[0]
	}
	if got.Value.Kind() != slog.KindGroup {
		t.Fatalf("level 8 = %v, want a group kept", got)
	}
	if child := got.Value.Group()[0]; child.Value.Kind() != slog.KindString || child.Value.String() != masked {
		t.Fatalf("level 9 = %v, want %q", child, masked)
	}
}

func TestRedact_masksAMapOnlyBelowTheEighthLevel(t *testing.T) {
	t.Parallel()
	var nested any = "deep-value"
	for range 12 {
		nested = map[string]any{"k": nested}
	}
	got := redact(slog.Any("top", nested))
	for level := range 8 {
		if got.Value.Kind() != slog.KindGroup || len(got.Value.Group()) != 1 {
			t.Fatalf("level %d = %v, want a group with one attribute", level, got)
		}
		got = got.Value.Group()[0]
	}
	if child := got.Value.Group()[0]; child.Value.Kind() != slog.KindString || child.Value.String() != masked {
		t.Fatalf("level 9 = %v, want %q", child, masked)
	}
}

func TestRedact_masksASliceElementOnlyBelowTheEighthLevel(t *testing.T) {
	t.Parallel()
	var nested any = "deep-value"
	for range 12 {
		nested = []any{nested}
	}
	level, ok := redact(slog.Any("top", nested)).Value.Any().([]any)
	for range 8 {
		if !ok || len(level) != 1 {
			t.Fatalf("slice level = %v, want one nested element", level)
		}
		level, ok = level[0].([]any)
	}
	if !ok || len(level) != 1 || level[0] != masked {
		t.Fatalf("level 9 = %v, want %q", level, masked)
	}
}
