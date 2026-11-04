//go:build faultpoints

package social_test

import (
	"bytes"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow22b_PostChatMessage_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	logs := &testkit.Logs{}
	flows.F22bPostChatMessageCrashBeforeCommit(chatScenario(t,
		scenario.WithLogs(logs), scenario.WithRequestMiddleware(crashFirstChatPost(faultpoint.BeforeCommit))))
	if !bytes.Contains(logs.Bytes(), []byte("faultpoint: crash at before-commit")) {
		t.Fatalf("the first chat post never crashed at before-commit; logs:\n%s", logs.Bytes())
	}
}
