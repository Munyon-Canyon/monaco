//go:build faultpoints

package cabal_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const standingChaosMembers = 6

func standingChaosID(kind string, i int) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte(kind+"-"+strconv.Itoa(i)))
}

func TestStanding_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedStandingCabal(t, h)
		for _, c := range cabal.New(module.Deps{
			Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus,
		}).Consumers() {
			if c.Durable == "cabal_standing" {
				return c
			}
		}
		t.Fatal("cabal_standing is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event {
		at := time.Date(2026, 3, 1, 12, 0, i, 0, time.UTC)
		if i%3 == 1 {
			return events.UserDeleted{V: 1, UserID: standingChaosID("user", 3+i/3), At: at}
		}
		user := standingChaosID("user", 1+i%2)
		return events.UserAccountStatusChanged{V: 1, UserID: user, From: "active", To: "banned", Cause: "admin", At: at}
	})
}

func seedStandingCabal(t *testing.T, h testkit.Harness) {
	t.Helper()
	at := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	cabalID := standingChaosID("cabal", 0)
	for i := range standingChaosMembers + 1 {
		id := standingChaosID("user", i)
		_, err := h.Pool.Exec(t.Context(), `INSERT INTO users
			(id, privy_user_id, login_provider, auth_state, auth_state_changed_at, account_status, created_at, updated_at)
			VALUES ($1, $2, 'sms', 'CREATED', $3, 'active', $3, $3)`, id, "did:privy:"+id.String(), at)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := h.Pool.Exec(t.Context(), `INSERT INTO cabals
		(id, name, creator_id, join_mode, voter_mode, threshold, proposal_expiry_seconds, created_at, updated_at)
		VALUES ($1, 'chaos cabal', $2, 'open', 'all', 'majority', 86400, $3, $3)`,
		cabalID, standingChaosID("user", 0), at)
	if err != nil {
		t.Fatal(err)
	}
	for i := range standingChaosMembers + 1 {
		role := "member"
		if i == 0 {
			role = "creator"
		}
		_, err := h.Pool.Exec(t.Context(), `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
			VALUES ($1, $2, $3, true, $4)`, cabalID, standingChaosID("user", i), role, at.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
}
