package app_test

import (
	"context"
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func activityBase() time.Time { return time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC) }

type activityWorld struct {
	testkit.Faults
	gone      ids.CabalID
	gen       *testkit.IDs
	viewer    ids.UserID
	user      ids.UserID
	good      ids.CabalID
	banned    ids.CabalID
	votes     []governanceport.CastVote
	proposals []governanceport.ProposedRow
	replies   []app.ReplyRow
	subjects  map[ids.ProposalID]governanceport.Subject
	tops      []rankingport.ClosedTop
	blocked   bool
	voteCalls int
}

func newActivityWorld() *activityWorld {
	gen := testkit.NewIDs(6474)
	return &activityWorld{
		gen: gen, viewer: ids.NewUserID(gen), user: ids.NewUserID(gen),
		good: ids.CabalIDFrom(gen.NewV7()), banned: ids.CabalIDFrom(gen.NewV7()),
		subjects: map[ids.ProposalID]governanceport.Subject{},
	}
}

func (w *activityWorld) proposalID() ids.ProposalID { return ids.ProposalIDFrom(w.gen.NewV7()) }

func (w *activityWorld) vote(at time.Time, cabal ids.CabalID, choice string) governanceport.CastVote {
	v := governanceport.CastVote{
		ProposalID: w.proposalID(), CabalID: cabal, Kind: "buy", Symbol: "NVDA", Choice: choice, CastAt: at,
	}
	w.votes = append(w.votes, v)
	return v
}

func (w *activityWorld) proposal(at time.Time, status governanceport.Status) governanceport.ProposedRow {
	p := governanceport.ProposedRow{
		ProposalID: w.proposalID(), CabalID: w.good, Kind: "buy", Symbol: "META", Status: status, Yes: 4, No: 1,
		CreatedAt: at,
	}
	w.proposals = append(w.proposals, p)
	return p
}

func (w *activityWorld) reply(at time.Time, body string) app.ReplyRow {
	proposal := w.proposalID()
	micros := uint64(1_500_000_000)
	w.subjects[proposal] = governanceport.Subject{CabalID: w.good, Kind: "buy", Symbol: "NVDA", USDCMicros: &micros}
	r := app.ReplyRow{ID: w.gen.NewV7(), ProposalID: proposal, Body: body, At: at}
	w.replies = append(w.replies, r)
	return r
}

func (w *activityWorld) VotesBy(
	_ context.Context, _ ids.UserID, after *governanceport.Cursor, limit int,
) ([]governanceport.CastVote, error) {
	w.voteCalls++
	if err := w.Check("VotesBy"); err != nil {
		return nil, err
	}
	rows := slices.Clone(w.votes)
	slices.SortFunc(rows, func(a, b governanceport.CastVote) int {
		return compareDesc(a.CastAt, a.ProposalID.UUID(), b.CastAt, b.ProposalID.UUID())
	})
	rows = slices.DeleteFunc(rows, func(r governanceport.CastVote) bool {
		return !beforeCursor(after, r.CastAt, r.ProposalID.UUID())
	})
	return rows[:min(limit, len(rows))], nil
}

func (w *activityWorld) ProposalsBy(
	_ context.Context, _ ids.UserID, after *governanceport.Cursor, limit int,
) ([]governanceport.ProposedRow, error) {
	if err := w.Check("ProposalsBy"); err != nil {
		return nil, err
	}
	rows := slices.Clone(w.proposals)
	slices.SortFunc(rows, func(a, b governanceport.ProposedRow) int {
		return compareDesc(a.CreatedAt, a.ProposalID.UUID(), b.CreatedAt, b.ProposalID.UUID())
	})
	rows = slices.DeleteFunc(rows, func(r governanceport.ProposedRow) bool {
		return !beforeCursor(after, r.CreatedAt, r.ProposalID.UUID())
	})
	return rows[:min(limit, len(rows))], nil
}

func (w *activityWorld) ProposalSubjects(
	_ context.Context, proposals []ids.ProposalID,
) (map[ids.ProposalID]governanceport.Subject, error) {
	if err := w.Check("ProposalSubjects"); err != nil {
		return nil, err
	}
	out := map[ids.ProposalID]governanceport.Subject{}
	for _, id := range proposals {
		if subject, ok := w.subjects[id]; ok {
			out[id] = subject
		}
	}
	return out, nil
}

func (w *activityWorld) Replies(
	_ context.Context, _ ids.UserID, after *governanceport.Cursor, limit int,
) ([]app.ReplyRow, error) {
	if err := w.Check("Replies"); err != nil {
		return nil, err
	}
	rows := slices.Clone(w.replies)
	slices.SortFunc(rows, func(a, b app.ReplyRow) int { return compareDesc(a.At, a.ID, b.At, b.ID) })
	rows = slices.DeleteFunc(rows, func(r app.ReplyRow) bool { return !beforeCursor(after, r.At, r.ID) })
	return rows[:min(limit, len(rows))], nil
}

func (w *activityWorld) BlockedEitherWay(context.Context, ids.UserID, ids.UserID) (bool, error) {
	return w.blocked, w.Check("BlockedEitherWay")
}

func (w *activityWorld) ClosedMonthTops(context.Context, ids.UserID) ([]rankingport.ClosedTop, error) {
	return w.tops, w.Check("ClosedMonthTops")
}

func (w *activityWorld) Cabals(_ context.Context, cabals []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	if err := w.Check("Cabals"); err != nil {
		return nil, err
	}
	out := map[ids.CabalID]cabalport.CabalView{}
	for _, id := range cabals {
		if id == w.gone {
			continue
		}
		status := cabalport.StatusActive
		if id == w.banned {
			status = cabalport.StatusBanned
		}
		out[id] = cabalport.CabalView{ID: id, Name: "AC Capital", Status: status}
	}
	return out, nil
}

func (w *activityWorld) query() *app.UserActivityQuery {
	cards := []identityport.UserCard{
		{ID: w.viewer, AccountStatus: "active"}, {ID: w.user, AccountStatus: "active"},
	}
	return &app.UserActivityQuery{Store: w, Users: fakes.NewIdentity(cards, nil), Cabals: w, Activity: w, Tops: w}
}

func compareDesc(aAt time.Time, aID uuid.UUID, bAt time.Time, bID uuid.UUID) int {
	if c := bAt.Compare(aAt); c != 0 {
		return c
	}
	return strings.Compare(bID.String(), aID.String())
}

func beforeCursor(after *governanceport.Cursor, at time.Time, id uuid.UUID) bool {
	if after == nil || at.Before(after.At) {
		return true
	}
	return at.Equal(after.At) && strings.Compare(id.String(), after.ID.String()) < 0
}

func (w *activityWorld) page(
	t *testing.T,
	viewer ids.UserID,
	cursor *app.ActivityPosition,
	limit int,
) app.ActivityPage {
	t.Helper()
	page, err := w.query().Run(t.Context(), viewer, w.user, cursor, limit)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	return page
}

func (w *activityWorld) walk(t *testing.T, viewer ids.UserID, limit int) []app.ActivityRow {
	t.Helper()
	var rows []app.ActivityRow
	var cursor *app.ActivityPosition
	for range 40 {
		page := w.page(t, viewer, cursor, limit)
		rows = append(rows, page.Items...)
		if page.Next == nil {
			return rows
		}
		parsed, err := app.ParseActivityPosition(page.Next.Encode())
		if err != nil {
			t.Fatalf("a cursor the server wrote does not parse: %v", err)
		}
		cursor = &parsed
	}
	t.Fatal("the cursor never ended")
	return nil
}

func kinds(rows []app.ActivityRow) []app.ActivityKind {
	out := make([]app.ActivityKind, len(rows))
	for i, r := range rows {
		out[i] = r.Kind
	}
	return out
}

func TestUserActivity_mergesTheFourSourcesNewestFirstAcrossTwoPages(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	proposed := w.proposal(activityBase().Add(4*time.Hour), governanceport.StatusPassed)
	voted := w.vote(activityBase().Add(3*time.Hour), w.good, "yes")
	replied := w.reply(activityBase().Add(2*time.Hour), "Concentration is a feature")
	w.tops = []rankingport.ClosedTop{
		{PeriodStart: time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC), Right: 11, Resolved: 12},
	}
	first := w.page(t, w.viewer, nil, 2)
	if got, want := kinds(
		first.Items,
	), []app.ActivityKind{
		app.ActivityProposed,
		app.ActivityVoted,
	}; !slices.Equal(got, want) ||
		first.Next == nil {
		t.Fatalf("first page = %v, next %v, want %v and a cursor", got, first.Next, want)
	}
	second := w.page(t, w.viewer, first.Next, 2)
	if got, want := kinds(
		second.Items,
	), []app.ActivityKind{
		app.ActivityReplied,
		app.ActivityToppedBoard,
	}; !slices.Equal(got, want) ||
		second.Next != nil {
		t.Fatalf("second page = %v, next %v, want %v and no cursor", got, second.Next, want)
	}
	if first.Items[0].ProposalID != proposed.ProposalID || first.Items[1].ProposalID != voted.ProposalID ||
		second.Items[0].CommentID != replied.ID {
		t.Errorf("rows = %+v %+v, want the seeded proposal, vote and comment", first.Items, second.Items)
	}
}

func TestUserActivity_aProposalCarriesItsStatusAndItsCountOnceDecided(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	w.proposal(activityBase().Add(4*time.Hour), governanceport.StatusPassed)
	w.proposal(activityBase().Add(3*time.Hour), governanceport.StatusOpen)
	rows := w.page(t, w.viewer, nil, 10).Items
	decided, open := rows[0], rows[1]
	if decided.Subject != "Buy META" || decided.Status != "passed" || decided.Result == nil ||
		*decided.Result != (app.ActivityResult{Yes: 4, No: 1}) || decided.Cabal.Name != "AC Capital" {
		t.Errorf("decided proposal = %+v, want Buy META passed 4-1 in AC Capital", decided)
	}
	if open.Status != "open" || open.Result != nil {
		t.Errorf("open proposal = %+v, want no result while open", open)
	}
}

func TestUserActivity_aReplyNamesItsProposalItsAmountAndItsCabal(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	w.reply(activityBase().Add(2*time.Hour), "Concentration is a feature")
	replied := w.page(t, w.viewer, nil, 10).Items[0]
	if replied.Body != "Concentration is a feature" || replied.Subject != "Buy NVDA" || replied.USDCMicros == nil ||
		*replied.USDCMicros != 1_500_000_000 || replied.Cabal.Name != "AC Capital" {
		t.Errorf("reply = %+v, want the body on Buy NVDA $1,500 in AC Capital", replied)
	}
}

func TestUserActivity_aMonthWinCarriesItsCountDatedAtTheMonthsClose(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	september := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	w.tops = []rankingport.ClosedTop{{PeriodStart: september, Right: 11, Resolved: 12}}
	top := w.page(t, w.viewer, nil, 10).Items[0]
	if top.Kind != app.ActivityToppedBoard || top.Right != 11 || top.Resolved != 12 ||
		!top.At.Equal(september.AddDate(0, 1, 0)) || !top.PeriodStart.Equal(september) {
		t.Errorf("topped board = %+v, want September 11 of 12 dated at the month's close", top)
	}
}

func TestUserActivity_sameTimestampRowsAcrossAPageBoundaryAreNeitherLostNorRepeated(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	at := activityBase()
	want := make([]uuid.UUID, 0, 4)
	for range 2 {
		want = append(want, w.vote(at, w.good, "yes").ProposalID.UUID())
	}
	want = append(want, w.proposal(at, governanceport.StatusOpen).ProposalID.UUID())
	want = append(want, w.reply(at, "same instant").ID)
	slices.SortStableFunc(want[:2], func(a, b uuid.UUID) int { return strings.Compare(b.String(), a.String()) })
	for _, limit := range []int{1, 2, 3} {
		rows := w.walk(t, w.viewer, limit)
		got := make([]uuid.UUID, len(rows))
		for i, r := range rows {
			got[i] = r.ProposalID.UUID()
			if r.Kind == app.ActivityReplied {
				got[i] = r.CommentID
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("limit %d walk = %v, want %v with none lost or repeated", limit, got, want)
		}
	}
}

func TestUserActivity_aFilteredBatchStillYieldsTheSourcesLaterVisibleRows(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	for i := range 4 {
		w.vote(activityBase().Add(time.Duration(10-i)*time.Hour), w.banned, "yes")
	}
	visible := w.vote(activityBase().Add(6*time.Hour), w.good, "no")
	proposed := w.proposal(activityBase().Add(5*time.Hour), governanceport.StatusOpen)
	page := w.page(t, w.viewer, nil, 2)
	if len(page.Items) != 2 || page.Items[0].ProposalID != visible.ProposalID ||
		page.Items[1].ProposalID != proposed.ProposalID {
		t.Fatalf("page = %+v, want the visible vote then the proposal", page.Items)
	}
	if page.Next != nil {
		t.Errorf("next = %v, want none once every source is exhausted", page.Next)
	}
	if w.voteCalls != 3 {
		t.Errorf("vote batches = %d, want 3 (two filtered, one with the visible row)", w.voteCalls)
	}
}

func TestUserActivity_aSourceFilteredPastTheBatchCapStillLetsTheFeedContinue(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	for i := range 12 {
		w.vote(activityBase().Add(time.Duration(40-i)*time.Hour), w.banned, "yes")
	}
	visible := w.vote(activityBase().Add(time.Hour), w.good, "yes")
	proposed := w.proposal(activityBase().Add(30*time.Hour), governanceport.StatusOpen)
	first := w.page(t, w.viewer, nil, 2)
	if first.Next == nil {
		t.Fatalf("first page = %+v with no cursor, want one while votes remain unread", first.Items)
	}
	rows := w.walk(t, w.viewer, 2)
	if len(rows) != 2 || rows[0].ProposalID != proposed.ProposalID || rows[1].ProposalID != visible.ProposalID {
		t.Fatalf("walk = %+v, want the proposal then the visible vote, each once", rows)
	}
}

func TestUserActivity_choiceAppearsOnlyOnTheMembersOwnActivity(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	w.vote(activityBase(), w.good, "no")
	if got := w.page(t, w.viewer, nil, 5).Items[0].Choice; got != "" {
		t.Errorf("another viewer sees choice %q, want none", got)
	}
	if got := w.page(t, w.user, nil, 5).Items[0].Choice; got != "no" {
		t.Errorf("the member sees choice %q, want no", got)
	}
}

func TestUserActivity_aBlockedPairGetsAnEmptyPageEitherWay(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	w.vote(activityBase(), w.good, "yes")
	w.blocked = true
	page := w.page(t, w.viewer, nil, 5)
	if len(page.Items) != 0 || page.Next != nil {
		t.Errorf("blocked page = %+v, want empty", page)
	}
	if own := w.page(t, w.user, nil, 5); len(own.Items) != 1 {
		t.Errorf("own page = %+v, want the vote: a member is never blocked from themselves", own)
	}
}

func TestUserActivity_dropsRowsFromABannedCabalAndRepliesOnUnknownProposals(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	w.vote(activityBase().Add(3*time.Hour), w.banned, "yes")
	kept := w.vote(activityBase().Add(2*time.Hour), w.good, "yes")
	orphan := w.reply(activityBase().Add(time.Hour), "gone")
	delete(w.subjects, orphan.ProposalID)
	rows := w.page(t, w.viewer, nil, 10).Items
	if len(rows) != 1 || rows[0].ProposalID != kept.ProposalID {
		t.Errorf("rows = %+v, want only the vote in the live cabal", rows)
	}
}

func TestUserActivity_cutsABodyOnAGraphemeBoundary(t *testing.T) {
	t.Parallel()
	zwj := string(rune(0x200d))
	family := strings.Join([]string{"\U0001F468", "\U0001F469", "\U0001F467", "\U0001F466"}, zwj)
	flags := "\U0001F1FA\U0001F1F8\U0001F1EB\U0001F1F7"
	for name, tt := range map[string]struct{ body, want string }{
		"short":              {"hello", "hello"},
		"exact":              {strings.Repeat("x", 140), strings.Repeat("x", 140)},
		"plain cut":          {strings.Repeat("x", 150), strings.Repeat("x", 140)},
		"emoji at the cut":   {strings.Repeat("x", 139) + family + "tail", strings.Repeat("x", 139) + family},
		"emoji past the cut": {strings.Repeat("x", 140) + family, strings.Repeat("x", 140)},
		"accent at the cut":  {strings.Repeat("x", 139) + "éz", strings.Repeat("x", 139) + "é"},
		"flags pair up":      {strings.Repeat("x", 138) + flags + "tail", strings.Repeat("x", 138) + flags},
		"skin tone":          {strings.Repeat("x", 139) + "\U0001F44D\U0001F3FDz", strings.Repeat("x", 139) + "\U0001F44D\U0001F3FD"},
	} {
		w := newActivityWorld()
		w.reply(activityBase(), tt.body)
		if got := w.page(t, w.viewer, nil, 5).Items[0].Body; got != tt.want {
			t.Errorf("%s: body = %q, want %q", name, got, tt.want)
		}
	}
}

func TestUserActivity_refusesAnUnknownUserABadLimitAndACursorItDidNotWrite(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	if _, err := w.query().
		Run(t.Context(), w.viewer, ids.NewUserID(w.gen), nil, 5); errs.CodeOf(
		err,
	) != errs.CodeUserNotFound {
		t.Errorf("unknown user err = %v, want user_not_found", err)
	}
	for _, limit := range []int{0, app.UserActivityPageMax + 1} {
		if _, err := w.query().
			Run(t.Context(), w.viewer, w.user, nil, limit); errs.CodeOf(
			err,
		) != errs.CodeInvalidInput {
			t.Errorf("limit %d err = %v, want invalid_input", limit, err)
		}
	}
	for _, raw := range []string{"", "!!", "bm9wZQ", "MS4wLm5vdC1hLXV1aWQ", "MS45LjAxOTIwMDAwLTAwMDAtNzAwMC04MDAwLTAwMDAwMDAwMDAwMQ"} {
		if _, err := app.ParseActivityPosition(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("cursor %q err = %v, want invalid_input", raw, err)
		}
	}
}

func TestUserActivity_unwiredPortsAnswerUpstreamUnavailable(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	q := w.query()
	q.Activity = app.UnwiredMemberActivity{}
	if _, err := q.Run(t.Context(), w.viewer, w.user, nil, 5); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Errorf("unwired governance err = %v, want upstream_unavailable", err)
	}
	q = w.query()
	q.Tops = app.UnwiredClosedTops{}
	if _, err := q.Run(t.Context(), w.viewer, w.user, nil, 5); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Errorf("unwired ranking err = %v, want upstream_unavailable", err)
	}
}

func TestUserActivity_aFailingPortFailsTheReadWithItsCode(t *testing.T) {
	t.Parallel()
	for _, op := range []string{
		"VotesBy", "ProposalsBy", "ProposalSubjects", "Replies", "BlockedEitherWay", "ClosedMonthTops", "Cabals",
	} {
		w := newActivityWorld()
		w.vote(activityBase().Add(time.Hour), w.good, "yes")
		w.proposal(activityBase().Add(2*time.Hour), governanceport.StatusOpen)
		w.reply(activityBase().Add(3*time.Hour), "hi")
		w.tops = []rankingport.ClosedTop{{PeriodStart: time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)}}
		w.Fail(op, errs.New(errs.CodeDBUnavailable, "test"))
		if _, err := w.query().Run(t.Context(), w.viewer, w.user, nil, 5); errs.CodeOf(err) != errs.CodeDBUnavailable {
			t.Errorf("%s failing: err = %v, want db_unavailable", op, err)
		}
	}
}

func TestUserActivity_anUnreadableAccountFailsTheRead(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	users := fakes.NewIdentity(nil, nil)
	users.Fail("UsersByID", errs.New(errs.CodeDBUnavailable, "test"))
	q := w.query()
	q.Users = users
	if _, err := q.Run(t.Context(), w.viewer, w.user, nil, 5); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("err = %v, want db_unavailable", err)
	}
}

func TestUserActivity_aCabalTheLookupDoesNotKnowDropsItsRows(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	w.gone = ids.CabalIDFrom(w.gen.NewV7())
	w.vote(activityBase().Add(2*time.Hour), w.gone, "yes")
	kept := w.vote(activityBase().Add(time.Hour), w.good, "yes")
	rows := w.page(t, w.viewer, nil, 5).Items
	if len(rows) != 1 || rows[0].ProposalID != kept.ProposalID {
		t.Errorf("rows = %+v, want only the vote in the known cabal", rows)
	}
}

func TestUserActivity_batchesThatFilterToFewerRowsStillFillThePageWithoutOverflowing(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	want := make([]uuid.UUID, 0, 3)
	w.vote(activityBase().Add(10*time.Hour), w.banned, "yes")
	for i := range 3 {
		want = append(want, w.vote(activityBase().Add(time.Duration(9-i)*time.Hour), w.good, "yes").ProposalID.UUID())
	}
	rows := w.walk(t, w.viewer, 2)
	got := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		got[i] = r.ProposalID.UUID()
	}
	if !slices.Equal(got, want) {
		t.Errorf("walk = %v, want %v with none lost or repeated", got, want)
	}
}

func TestUserActivity_monthWinsPageNewestFirstAndSkipTheOnesAlreadySeen(t *testing.T) {
	t.Parallel()
	w := newActivityWorld()
	august := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	september := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	w.tops = []rankingport.ClosedTop{
		{PeriodStart: august, Right: 1, Resolved: 2},
		{PeriodStart: september, Right: 3, Resolved: 4},
	}
	rows := w.walk(t, w.viewer, 1)
	if len(rows) != 2 || !rows[0].PeriodStart.Equal(september) || !rows[1].PeriodStart.Equal(august) {
		t.Errorf("rows = %+v, want September then August", rows)
	}
}

func TestUserActivity_theUnwiredGovernancePortFailsEveryMethod(t *testing.T) {
	t.Parallel()
	var port app.UnwiredMemberActivity
	if _, err := port.ProposalsBy(t.Context(), ids.UserID{}, nil, 1); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Errorf("ProposalsBy err = %v, want upstream_unavailable", err)
	}
	if _, err := port.ProposalSubjects(t.Context(), nil); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Errorf("ProposalSubjects err = %v, want upstream_unavailable", err)
	}
}

func TestParseActivityPosition_refusesAFieldItDidNotWrite(t *testing.T) {
	t.Parallel()
	id := "01920000-0000-7000-8000-000000000001"
	for name, raw := range map[string]string{
		"no fields":    "1",
		"bad time":     "x.0." + id,
		"bad rank":     "1.x." + id,
		"rank too big": "1.9." + id,
		"bad date":     "1.3.2026-13-45",
		"bad id":       "1.0.nope",
	} {
		if _, err := app.ParseActivityPosition(
			base64.RawURLEncoding.EncodeToString([]byte(raw)),
		); errs.CodeOf(
			err,
		) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
	if _, err := app.ParseActivityPosition(base64.RawURLEncoding.EncodeToString([]byte("1.3.2026-09-01"))); err != nil {
		t.Errorf("a month cursor err = %v, want none", err)
	}
}
