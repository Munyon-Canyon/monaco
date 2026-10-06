package social_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	chatPrivyAppID = "app-fixture"
	chatAblyKey    = "flow22.key:flow22-secret"
)

func chatScenario(t *testing.T, extra ...scenario.Option) *scenario.Scenario {
	t.Helper()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	cfg := config.Config{
		Privy: config.Privy{
			AppID: chatPrivyAppID, AppSecret: "test-secret", BaseURL: srv.URL + "/privy",
			VerificationKey:         fakes.PrivyVerificationKey(),
			AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
			AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
		},
		Ably:     config.Ably{APIKey: chatAblyKey, RESTHost: srv.URL + "/ably"},
		Timeouts: config.Timeouts{Privy: 10 * time.Second, Ably: 5 * time.Second},
	}
	withCfg := func(build func(module.Deps) module.Module) func(module.Deps) module.Module {
		return func(d module.Deps) module.Module {
			d.Config = cfg
			d.HTTPClient = httpclient.New
			return build(d)
		}
	}
	return scenario.New(t, append([]scenario.Option{
		scenario.WithModules(
			withCfg(func(d module.Deps) module.Module { return identity.New(d) }),
			withCfg(func(d module.Deps) module.Module { return cabal.New(d) }),
			withCfg(func(d module.Deps) module.Module { return social.New(d) }),
		),
		scenario.WithPrivy(upstreams, chatPrivyAppID),
	}, extra...)...)
}

func TestFlow22_PostChatMessage_OK(t *testing.T) {
	t.Parallel()
	flows.F22PostChatMessageOK(chatScenario(t))
}

func TestPostChatMessage_aFailedAblyPublishStillReturns201(t *testing.T) {
	t.Parallel()
	flows.ChatPostSurvivesAblyDown(chatScenario(t))
}

func TestFlow22_PostChatMessage_NotCabalMember(t *testing.T) {
	t.Parallel()
	flows.F22PostChatMessageNotCabalMember(chatScenario(t))
}

func TestFlow22_PostChatMessage_ChatParentNotFound(t *testing.T) {
	t.Parallel()
	flows.F22PostChatMessageChatParentNotFound(chatScenario(t))
}

func TestFlow22_PostChatMessage_ChatParentIsReply(t *testing.T) {
	t.Parallel()
	flows.F22PostChatMessageChatParentIsReply(chatScenario(t))
}

func TestFlow22_PostChatMessage_ChatBodyInvalid(t *testing.T) {
	t.Parallel()
	flows.F22PostChatMessageChatBodyInvalid(chatScenario(t))
}

func TestFlow22_PostChatMessage_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F22PostChatMessageUnauthorized(chatScenario(t))
}

func TestFlow22_DeleteChatMessage_OK(t *testing.T) {
	t.Parallel()
	flows.F22DeleteChatMessageOK(chatScenario(t))
}

func TestFlow22_DeleteChatMessage_ChatMessageNotOwned(t *testing.T) {
	t.Parallel()
	flows.F22DeleteChatMessageChatMessageNotOwned(chatScenario(t))
}

func TestFlow22_DeleteChatMessage_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F22DeleteChatMessageUnauthorized(chatScenario(t))
}
