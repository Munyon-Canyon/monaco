package analytics

import (
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
	maxKeyChars = 44
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
	for _, key := range slices.Sorted(maps.Keys(node)) {
		at := key
		if path != "" {
			at = path + "." + key
		}
		if bannedKey(key) {
			return refuse(op, at, "key")
		}
		if err := scan(op, at, node[key]); err != nil {
			return err
		}
	}
	return nil
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

func piiValue(s string) string {
	switch {
	case emailValue.MatchString(s):
		return "email"
	case phoneValue.MatchString(s):
		return "phone"
	case solanaKey(s):
		return "wallet_key"
	}
	return ""
}

func solanaKey(s string) bool {
	if len(s) < minKeyChars || len(s) > maxKeyChars {
		return false
	}
	b, ok := chain.DecodeBase58(s)
	return ok && len(b) == keyBytes
}
