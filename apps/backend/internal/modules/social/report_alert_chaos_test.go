//go:build faultpoints

package social_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestReportAlert_Chaos_ConvergesToTheSingleDeliveryRun(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		deps := module.Deps{Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
		for _, c := range social.New(deps).Consumers() {
			if c.Durable == "social_report_alert" {
				return c
			}
		}
		t.Fatal("social_report_alert is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event {
		return events.ReportCreated{
			V: 1, ReportID: uuid.NewSHA1(uuid.Nil, []byte("report-chaos"+strconv.Itoa(i))),
			ReporterID: uuid.NewSHA1(uuid.Nil, []byte("report-chaos-reporter")),
			Kind:       "message", TargetID: uuid.NewSHA1(uuid.Nil, []byte("report-chaos-target"+strconv.Itoa(i))),
			Reason: "abuse",
		}
	})
}
