package bus

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func TestConsumeError_warnsWithTheConsumerAndTheError(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, &logs))

	consumeError(ctx, "notify", jetstream.ErrConsumerDeleted)
	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	if line["msg"] != "bus.consume_error" || line["level"] != "WARN" || line["consumer"] != "notify" ||
		line["err"] != jetstream.ErrConsumerDeleted.Error() {
		t.Fatalf("line = %v", line)
	}
}
