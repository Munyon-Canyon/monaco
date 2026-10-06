package analytics

import (
	"crypto/ed25519"
	"encoding/json"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	minKeyChars = 32
	maxSigChars = 88
	keyBytes    = 32
)

var (
	emailValue = regexp.MustCompile(`[^\s@]+@[^\s@]+\.[^\s@]+`)
	phoneValue = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
)

func CheckNoPII(c Capture) error {
	const op = "analytics.CheckNoPII"
	raw, err := json.Marshal(map[string]any{
		"event": c.Event, "distinct_id": c.DistinctID, "properties": c.Properties, "set": c.Set,
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeAnalyticsPII, op, slog.String("reason", "unencodable"))
	}
	var tree any
	_ = json.Unmarshal(raw, &tree)
	return scan(op, "", tree)
}

func scan(op, path string, node any) error {
	switch node := node.(type) {
	case map[string]any:
		return scanMap(op, path, node)
	case []any:
		return scanList(op, path, node)
	case string:
		if kind := piiValue(node); kind != "" {
			return refuse(op, path, kind)
		}
	}
	return nil
}

func scanMap(op, path string, node map[string]any) error {
	for i, key := range slices.Sorted(maps.Keys(node)) {
		if kind := piiValue(key); kind != "" {
			return refuse(op, join(path, "{"+strconv.Itoa(i)+"}"), keyReason(kind))
		}
		at := join(path, key)
		if bannedKey(key) {
			return refuse(op, at, "key")
		}
		if err := scan(op, at, node[key]); err != nil {
			return err
		}
	}
	return nil
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func scanList(op, path string, node []any) error {
	for i, item := range node {
		if err := scan(op, path+"["+strconv.Itoa(i)+"]", item); err != nil {
			return err
		}
	}
	return nil
}

func refuse(op, path, reason string) error {
	return errs.New(errs.CodeAnalyticsPII, op, slog.String("path", path), slog.String("reason", reason))
}

func bannedKey(key string) bool {
	switch strings.ToLower(key) {
	case "email", "phone", "x_handle", "handle", "display_name", "wallet", "wallet_address", "address",
		"to_address", "signature", "tx_signature", "mint":
		return true
	}
	return false
}

func keyReason(kind string) string {
	if kind == "wallet_key" {
		return "key_wallet"
	}
	return "key_" + kind
}

func piiValue(s string) string {
	switch {
	case emailValue.MatchString(s):
		return "email"
	case phoneValue.MatchString(s):
		return "phone"
	}
	return solanaValue(s)
}

func solanaValue(s string) string {
	if len(s) < minKeyChars || len(s) > maxSigChars {
		return ""
	}
	b, ok := chain.DecodeBase58(s)
	if !ok {
		return ""
	}
	switch len(b) {
	case keyBytes:
		return "wallet_key"
	case ed25519.SignatureSize:
		return "signature"
	}
	return ""
}
