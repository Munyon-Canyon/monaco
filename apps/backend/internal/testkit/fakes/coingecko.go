package fakes

import "net/http"

func foldedCoinGeckoRoute(upstream string, r *http.Request) (string, []string, bool) {
	days := r.URL.Query().Get("days")
	if upstream != "coingecko" || days == "" {
		return "", nil, false
	}
	route := "/" + upstream + r.URL.Path
	return route, []string{route + "/" + days, route}, true
}
