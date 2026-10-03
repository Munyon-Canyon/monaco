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

func DisplayName(issuer Issuer, raw string) string {
	name := strings.TrimSpace(raw)
	a := affixOf(issuer)
	if a.prefix != "" && len(name) > len(a.prefix) && strings.EqualFold(name[:len(a.prefix)], a.prefix) {
		name = strings.TrimSpace(name[len(a.prefix):])
	}
	if cut := len(name) - len(a.suffix); a.suffix != "" && cut > 0 && strings.EqualFold(name[cut:], a.suffix) {
		name = strings.TrimSpace(name[:cut])
	}
	return name
}

func CompanyKey(issuer Issuer, raw string) string {
	return strings.ToLower(DisplayName(issuer, raw))
}
