package apns_test

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestNoopSender_answersOKAndLogsOnlyTheUserID(t *testing.T) {
	t.Parallel()
	user, err := ids.ParseUserID(testkit.NewIDs(1).NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	p := push(apns.Production)
	p.UserID = user

	res, err := apns.NoopSender{}.Send(ctx, p)

	if err != nil || res != (apns.Result{Status: http.StatusOK}) || apns.Classify(res) != apns.Delivered {
		t.Fatalf("Send = %+v, %v, want a delivered 200", res, err)
	}
	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &line); err != nil {
		t.Fatalf("log output %q is not one JSON line: %v", logs.Bytes(), err)
	}
	if keys := slices.Sorted(maps.Keys(line)); !slices.Equal(keys, []string{"level", "msg", "time", "user_id"}) ||
		line["msg"] != "apns.noop_send" || line["user_id"] != user.String() {
		t.Fatalf("log line = %v, want only the message and the user id", line)
	}
}
