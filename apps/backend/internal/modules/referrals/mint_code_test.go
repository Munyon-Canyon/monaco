package referrals_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func deliveredAt() time.Time { return time.Date(2026, 3, 1, 12, 0, 1, 0, time.UTC) }

type mintFixture struct {
	pool  *pgxpool.Pool
	uow   *db.UnitOfWork
	users *testkit.IDs
}

func newMintFixture(t *testing.T) mintFixture {
	t.Helper()
	pool := testkit.DB(t)
	return mintFixture{
		pool:  pool,
		uow:   db.New(pool, testkit.NewIDs(1), testkit.NewClock(deliveredAt())),
		users: testkit.NewIDs(2),
	}
}

func (f mintFixture) mint(t *testing.T, random io.Reader, user uuid.UUID) error {
	t.Helper()
	return f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		h := adapters.MintCode{Entropy: func(context.Context) io.Reader { return random }}
		return h.Handle(ctx, tx, events.UserCreated{V: 1, UserID: user}, deliveredAt())
	})
}

func (f mintFixture) codes(t *testing.T) map[uuid.UUID]string {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT user_id, code FROM referral_codes`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[uuid.UUID]string{}
	for rows.Next() {
		var (
			user uuid.UUID
			code string
		)
		if err := rows.Scan(&user, &code); err != nil {
			t.Fatal(err)
		}
		out[user] = code
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f mintFixture) seedCode(t *testing.T, code string) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO referral_codes (code, user_id, created_at) VALUES ($1, $2, now())`, code, f.users.NewV7(),
	); err != nil {
		t.Fatal(err)
	}
}

func draws(values ...byte) io.Reader {
	b := make([]byte, 0, 8*len(values))
	for _, v := range values {
		b = append(b, bytes.Repeat([]byte{v}, 8)...)
	}
	return bytes.NewReader(b)
}

func TestMintCode_replayingThreeUserCreatedEventsLeavesThreeCodes(t *testing.T) {
	t.Parallel()
	f := newMintFixture(t)
	users := []uuid.UUID{f.users.NewV7(), f.users.NewV7(), f.users.NewV7()}
	random := draws(0, 1, 2, 3, 4, 5)
	for _, user := range append(users, users[2], users[0], users[1]) {
		if err := f.mint(t, random, user); err != nil {
			t.Fatal(err)
		}
	}
	want := map[uuid.UUID]string{users[0]: "22222222", users[1]: "33333333", users[2]: "44444444"}
	if got := f.codes(t); len(got) != 3 || got[users[0]] != want[users[0]] ||
		got[users[1]] != want[users[1]] || got[users[2]] != want[users[2]] {
		t.Fatalf("codes after replay = %v, want %v", got, want)
	}
}

func TestMintCode_retriesWithANewCodeWhenTheCodeIsTaken(t *testing.T) {
	t.Parallel()
	f := newMintFixture(t)
	f.seedCode(t, "22222222")
	f.seedCode(t, "33333333")
	user := f.users.NewV7()
	if err := f.mint(t, draws(0, 1, 2), user); err != nil {
		t.Fatal(err)
	}
	if got := f.codes(t)[user]; got != "44444444" {
		t.Fatalf("code = %q, want 44444444 after two collisions", got)
	}
}

func TestMintCode_failsInternalAfterFiveRetriesAndMintsNothing(t *testing.T) {
	t.Parallel()
	f := newMintFixture(t)
	f.seedCode(t, "22222222")
	random := bytes.NewReader(bytes.Repeat([]byte{0}, 8*(adapters.MintRetries+2)))
	user := f.users.NewV7()
	err := f.mint(t, random, user)
	if errs.CodeOf(err) != errs.CodeInternal || random.Len() != 8 {
		t.Fatalf("mint = %v with %d bytes unread; want internal after exactly six draws", err, random.Len())
	}
	if code, ok := f.codes(t)[user]; ok {
		t.Fatalf("user has code %q, want none", code)
	}
}

func TestMintCode_returnsTheEntropyAndWriteErrors(t *testing.T) {
	t.Parallel()
	f := newMintFixture(t)
	if err := f.mint(t, bytes.NewReader(nil), f.users.NewV7()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("mint with no entropy = %v, want internal", err)
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE referral_codes RENAME TO referral_codes_gone`); err != nil {
		t.Fatal(err)
	}
	if err := f.mint(t, draws(0), f.users.NewV7()); err == nil {
		t.Fatal("mint without the referral_codes table succeeded")
	}
}
