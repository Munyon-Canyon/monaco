package testkit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
)

func DeployedEnv() []string {
	return append(APNsEnv(), "ABLY_API_KEY=deployed.key:deployed-secret",
		"APP_STORE_URL=https://apps.apple.com/app/id6820853661",
		"AGENT_KEY_ENCRYPTION_KEY=AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=",
		"PRIVY_AUTHORIZATION_PRIVATE_KEY="+deployedAuthorizationKey(), "PRIVY_AUTHORIZATION_KEY_ID=deployed-key-quorum")
}

func deployedAuthorizationKey() string {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	return "wallet-auth:" + base64.StdEncoding.EncodeToString(der)
}
