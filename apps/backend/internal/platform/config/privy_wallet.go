package config

import "net/url"

const realPrivyHost = "api.privy.io"

func (e *keysError) checkPrivyWallet(c Config) {
	if !c.Env.Deployed() {
		return
	}
	if c.Privy.WalletBaseURL != "" {
		e.invalid = append(e.invalid, "PRIVY_WALLET_BASE_URL (not allowed in staging or production)")
	}
	if c.Trade.Engine == TradeEngineLive && !realPrivy(c.Privy.WalletURL()) {
		e.invalid = append(e.invalid, "PRIVY_BASE_URL (live trading in staging or production needs the real Privy API)")
	}
}

func realPrivy(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == realPrivyHost
}
