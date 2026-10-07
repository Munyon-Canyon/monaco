//go:build faultpoints

package admin_test

import (
	"encoding/json"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestAudit_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		for _, c := range admin.New(module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}).Consumers() {
			if c.Durable == "admin" {
				return c
			}
		}
		t.Fatal("the admin durable is not registered")
		return bus.Consumer{}
	}, func(rng *rand.Rand, i int) events.Event {
		const before, after = `{"flagged_at":null}`, `{"flagged_at":"2026-03-01T12:00:00Z"}`
		action := uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10))
		admin := uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10))
		e := events.AdminAction{
			V: 1, ActionID: action, AdminID: admin, Action: events.AdminActionPingFlag,
			TargetType: events.AdminTargetSystemPing, TargetID: strconv.Itoa(i), Reason: "spam",
			Before: json.RawMessage(before), After: json.RawMessage(after),
		}
		if i%2 == 1 {
			e.ApprovedBy = &admin
		}
		return e
	})
}
