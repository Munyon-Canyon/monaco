package testkit

func DeployedEnv() []string {
	return append(APNsEnv(), "ABLY_API_KEY=deployed.key:deployed-secret")
}
