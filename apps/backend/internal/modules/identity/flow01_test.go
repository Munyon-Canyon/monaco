package identity_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func identityScenario(t *testing.T, extra ...scenario.Option) *scenario.Scenario {
	t.Helper()
	client, _, upstreams := overPrivyFakes(t)
	users, wallets := privyadapter.Users{Client: client}, privyadapter.Wallets{Client: client}
	return scenario.New(t, append([]scenario.Option{
		scenario.WithModules(func(d module.Deps) module.Module {
			d.Config.Identity.NudgesInterval = time.Second
			return identity.New(d, identity.WithPrivy(users, wallets))
		}),
		scenario.WithPrivy(upstreams, privyAppID),
	}, extra...)...)
}

func TestFlow01_OpenSession_OK(t *testing.T) {
	t.Parallel()
	flows.F01OpenSessionOK(identityScenario(t))
}

func TestFlow01_OpenSession_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F01OpenSessionUnauthorized(identityScenario(t))
}

func TestFlow01_OpenSession_LoginMethodNotAllowed(t *testing.T) {
	t.Parallel()
	flows.F01OpenSessionLoginMethodNotAllowed(identityScenario(t))
}

func TestFlow01_OpenSession_AccountDeleted(t *testing.T) {
	t.Parallel()
	flows.F01OpenSessionAccountDeleted(identityScenario(t))
}

func TestFlow01_OpenSession_PrivyUnavailable(t *testing.T) {
	t.Parallel()
	flows.F01OpenSessionPrivyUnavailable(identityScenario(t))
}

func TestFlow01a_SetHandle_OK(t *testing.T) {
	t.Parallel()
	flows.F01aSetHandleOK(identityScenario(t))
}

func TestFlow01a_SetHandle_HandleInvalid(t *testing.T) {
	t.Parallel()
	flows.F01aSetHandleHandleInvalid(identityScenario(t))
}

func TestFlow01a_SetHandle_HandleReserved(t *testing.T) {
	t.Parallel()
	flows.F01aSetHandleHandleReserved(identityScenario(t))
}

func TestFlow01a_SetHandle_HandleTaken(t *testing.T) {
	t.Parallel()
	flows.F01aSetHandleHandleTaken(identityScenario(t))
}

func TestFlow01a_SetHandle_HandleTooSoon(t *testing.T) {
	t.Parallel()
	flows.F01aSetHandleHandleTooSoon(identityScenario(t))
}

func onboardingPII() []string {
	phones := []string{"+14155550111", "+14155550112", "+14155550113", "+15550000000"}
	pii := append(slices.Clone(phones), "9100000001", "9100000002", "9100000003")
	for _, phone := range phones {
		sum := sha256.Sum256([]byte(phone))
		pii = append(pii, hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(sum[:]))
	}
	return pii
}

func onboardingFlow(t *testing.T, script func(*scenario.Scenario)) {
	t.Helper()
	logs := &testkit.Logs{}
	script(identityScenario(t, scenario.WithLogs(logs)))
	for _, v := range onboardingPII() {
		if bytes.Contains(logs.Bytes(), []byte(v)) {
			t.Fatalf("logs carry %q:\n%s", v, logs.Bytes())
		}
	}
}

func TestFlow01b_LinkPhone_OK(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01bLinkPhoneOK)
}

func TestFlow01b_LinkPhone_HandleRequired(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01bLinkPhoneHandleRequired)
}

func TestFlow01b_LinkPhone_PhoneNotLinked(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01bLinkPhonePhoneNotLinked)
}

func TestFlow01b_LinkPhone_PrivyUnavailable(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01bLinkPhonePrivyUnavailable)
}

func TestFlow01c_LinkSocials_OK(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01cLinkSocialsOK)
}

func TestFlow01c_LinkSocials_HandleRequired(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01cLinkSocialsHandleRequired)
}

func TestFlow01c_LinkSocials_XNotLinked(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01cLinkSocialsXNotLinked)
}

func TestFlow01c_LinkSocials_PrivyUnavailable(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01cLinkSocialsPrivyUnavailable)
}

func TestFlow01d_SkipOnboardingStep_OK(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01dSkipOnboardingStepOK)
}

func TestFlow01d_SkipOnboardingStep_HandleRequired(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01dSkipOnboardingStepHandleRequired)
}

func TestFlow01d_SkipOnboardingStep_InvalidInput(t *testing.T) {
	t.Parallel()
	onboardingFlow(t, flows.F01dSkipOnboardingStepInvalidInput)
}
