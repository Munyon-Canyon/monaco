package domain

import "strings"

type affix struct {
	prefix, suffix string
}

func affixOf(issuer Issuer) affix {
	switch issuer {
	case IssuerXStocks:
		return affix{suffix: " xstock"}
	case IssuerTessera:
		return affix{prefix: "t-"}
	case IssuerPreStocks:
		return affix{suffix: " prestocks"}
	}
	return affix{}
}

func cleanName(issuer Issuer, raw string) string {
	name := strings.TrimSpace(raw)
	a := affixOf(issuer)
	if a.prefix != "" && len(name) >= len(a.prefix) && strings.EqualFold(name[:len(a.prefix)], a.prefix) {
		name = strings.TrimSpace(name[len(a.prefix):])
	}
	if a.suffix != "" {
		if strings.EqualFold(name, strings.TrimSpace(a.suffix)) {
			name = ""
		} else if cut := len(name) - len(a.suffix); cut >= 0 && strings.EqualFold(name[cut:], a.suffix) {
			name = strings.TrimSpace(name[:cut])
		}
	}
	return name
}

func DisplayName(issuer Issuer, raw, symbol string) string {
	if name := cleanName(issuer, raw); name != "" {
		return name
	}
	return symbol
}

func CompanyKeyForAsset(issuer Issuer, raw, symbol string) string {
	return strings.ToLower(DisplayName(issuer, raw, symbol))
}

func CompanyKey(issuer Issuer, raw string) string {
	return strings.ToLower(cleanName(issuer, raw))
}
