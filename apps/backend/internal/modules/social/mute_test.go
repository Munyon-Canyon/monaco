package social_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
)

func (f fixture) feedObject(
	t *testing.T, id uuid.UUID, kind, title string, cabal, asset, actor uuid.UUID, name, symbol, actorName string,
) {
	t.Helper()
	query := `INSERT INTO feed_objects
		(id, kind, ref_type, ref_id, cabal_id, cabal_name, asset_id, actor_id, symbol, title, payload, created_at, updated_at)
		VALUES ($1, $2, 'swaps', $3, $4, $5, $6, $7, $8, $9, jsonb_build_object('actor_name', $10::text), $11, $11)`
	_, err := f.pool.Exec(
		t.Context(), query, id, kind, f.gen.NewV7(), nullableUUID(cabal), nullableText(name), nullableUUID(asset),
		nullableUUID(actor), nullableText(symbol), title, actorName, f.now,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func nullableUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }
func nullableText(s string) pgtype.Text     { return pgtype.Text{String: s, Valid: s != ""} }

func TestMute_snapshotsEachTargetLabelAndIsIdempotent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal, asset, actor, item := f.gen.NewV7(), f.gen.NewV7(), f.bob.UUID(), f.gen.NewV7()
	f.feedObject(t, item, "trade", "Alpha bought Apple", cabal, asset, actor, "Alpha", "AAPLx", "Bob")
	tests := []struct{ typ, id, want string }{
		{"kind", "trade", "Trades"},
		{"cabal", cabal.String(), "Alpha"},
		{"asset", asset.String(), "AAPLx"},
		{"user", actor.String(), "Bob"},
		{"item", item.String(), "Alpha bought Apple"},
	}
	for _, tt := range tests {
		for range 2 {
			cmd := app.Mute{User: f.alice, TargetType: tt.typ, TargetID: tt.id}
			if err := f.mute.Handle(f.ctx(t), cmd); err != nil {
				t.Fatal(err)
			}
		}
		var label string
		query := `SELECT label FROM feed_mutes WHERE user_id = $1 AND target_type = $2 AND target_id = $3`
		err := f.pool.QueryRow(t.Context(), query, f.alice.UUID(), tt.typ, tt.id).Scan(&label)
		if err != nil || label != tt.want {
			t.Fatalf("%s label = %q, %v; want %q", tt.typ, label, err, tt.want)
		}
	}
	var count int
	row := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM feed_mutes WHERE user_id = $1`, f.alice.UUID())
	err := row.Scan(&count)
	if err != nil || count != len(tests) {
		t.Fatalf("count = %d, %v; want %d", count, err, len(tests))
	}
}

func TestMute_rejectsUnknownAndSelfAndUnmuteIsIdempotent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, cmd := range []app.Mute{
		{User: f.alice, TargetType: "kind", TargetID: "nope"},
		{User: f.alice, TargetType: "user", TargetID: f.alice.String()},
		{User: f.alice, TargetType: "asset", TargetID: f.gen.NewV7().String()},
		{User: f.alice, TargetType: "unknown", TargetID: "anything"},
		{User: f.alice, TargetType: "item", TargetID: "not-a-uuid"},
	} {
		if err := f.mute.Handle(f.ctx(t), cmd); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("err = %v", err)
		}
	}
	for range 2 {
		cmd := app.Unmute{User: f.alice, TargetType: "kind", TargetID: string(feed.KindTrade)}
		if err := f.unmute.Handle(f.ctx(t), cmd); err != nil {
			t.Fatal(err)
		}
	}
	invalid := app.Unmute{User: f.alice, TargetType: "unknown"}
	if err := f.unmute.Handle(f.ctx(t), invalid); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE feed_mutes`); err != nil {
		t.Fatal(err)
	}
	mute := app.Mute{User: f.alice, TargetType: "kind", TargetID: "trade"}
	if err := f.mute.Handle(f.ctx(t), mute); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v", err)
	}
	unmute := app.Unmute{User: f.alice, TargetType: "kind", TargetID: "trade"}
	if err := f.unmute.Handle(f.ctx(t), unmute); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v", err)
	}
}
