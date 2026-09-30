package testkit

import "embed"

//go:embed testdata/apns/test_key.p8
var apnsKey embed.FS

func APNsKeyP8() string {
	raw, _ := apnsKey.ReadFile("testdata/apns/test_key.p8")
	return string(raw)
}

func APNsEnv() []string {
	return []string{"APNS_KEY_P8=" + APNsKeyP8(), "APNS_KEY_ID=TESTKEYID1", "APNS_TEAM_ID=TESTTEAMID"}
}
