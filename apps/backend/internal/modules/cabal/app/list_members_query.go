package app

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	listMembersOp = "cabal.ListMembers"

	ListMembersDefault = 50
	listMembersMax     = 200

	OrderRoleShare = "role_share"
	OrderJoined    = "joined"
)

type ListMembersQuery struct {
	CabalID ids.CabalID
	Limit   int
	Cursor  string
	Order   string
}

type MemberPage struct {
	Items      []Member
	NextCursor string
	Totals     domain.MemberTotals
}

type MemberOwnership interface {
	Ownership(ctx context.Context, cabalID ids.CabalID) (map[ids.UserID]int32, error)
}

type MemberCalls interface {
	Records(
		ctx context.Context, userIDs []ids.UserID, cabalID *ids.CabalID, p rankingport.Period,
	) (map[ids.UserID]rankingport.CallRecord, error)
}

type MemberReads struct {
	Ownership MemberOwnership
	Calls     MemberCalls
	Now       func() time.Time
}

type NoOwnership struct{}

func (NoOwnership) Ownership(context.Context, ids.CabalID) (map[ids.UserID]int32, error) {
	return map[ids.UserID]int32{}, nil
}

type NoCalls struct{}

func (NoCalls) Records(
	context.Context, []ids.UserID, *ids.CabalID, rankingport.Period,
) (map[ids.UserID]rankingport.CallRecord, error) {
	return map[ids.UserID]rankingport.CallRecord{}, nil
}

func ListMembersPage(
	ctx context.Context, q sqlc.DBTX, users UserCards, reads MemberReads, req ListMembersQuery,
) (MemberPage, error) {
	byShare, cursor, err := checkMembersQuery(req)
	if err != nil {
		return MemberPage{}, err
	}
	dbq := sqlc.New(q)
	if _, err := dbq.FindCabal(ctx, req.CabalID.UUID()); errors.Is(err, sql.ErrNoRows) {
		return MemberPage{}, errs.New(errs.CodeCabalNotFound, listMembersOp)
	} else if err != nil {
		return MemberPage{}, errs.Wrap(err, errs.CodeInternal, listMembersOp)
	}
	members, userIDs, err := orderedMembers(ctx, dbq, reads.Ownership, req.CabalID, byShare)
	if err != nil {
		return MemberPage{}, err
	}
	records, err := monthRecords(ctx, reads, req.CabalID, userIDs)
	if err != nil {
		return MemberPage{}, err
	}
	start, end := pageBounds(members, cursor, byShare, req.Limit)
	page := MemberPage{Items: []Member{}, Totals: domain.TotalMembers(members)}
	if end < len(members) {
		page.NextCursor = encodeMembersCursor(memberKey(members[end-1], byShare), byShare)
	}
	if start == end {
		return page, nil
	}
	pageIDs := make([]ids.UserID, 0, end-start)
	for _, m := range members[start:end] {
		pageIDs = append(pageIDs, ids.UserIDFrom(m.UserID))
	}
	cards, err := users.UsersByID(ctx, pageIDs)
	if err != nil {
		return MemberPage{}, errs.Wrap(err, errs.CodeInternal, listMembersOp)
	}
	crown := domain.CrownHolder(members, records)
	for _, m := range members[start:end] {
		rec := records[m.UserID]
		page.Items = append(page.Items, Member{
			Person: person(m.UserID, cards), Role: string(m.Role), CanVote: m.CanVote, JoinedAt: m.Joined,
			OwnershipBps: m.Bps, Month: rec, Crown: m.UserID == crown,
		})
	}
	return page, nil
}

func orderedMembers(
	ctx context.Context, dbq *sqlc.Queries, owners MemberOwnership, cabalID ids.CabalID, byShare bool,
) ([]domain.OrderedMember, []ids.UserID, error) {
	rows, err := dbq.ListMembers(ctx, cabalID.UUID())
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeInternal, listMembersOp)
	}
	shares, err := owners.Ownership(ctx, cabalID)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeInternal, listMembersOp)
	}
	members := make([]domain.OrderedMember, len(rows))
	userIDs := make([]ids.UserID, len(rows))
	for i, row := range rows {
		userIDs[i] = ids.UserIDFrom(row.UserID)
		members[i] = domain.OrderedMember{
			UserID: row.UserID, Role: domain.Role(row.Role), CanVote: row.CanVote, Joined: row.JoinedAt,
			Bps: shares[userIDs[i]],
		}
	}
	if byShare {
		domain.MemberOrder(members)
	}
	return members, userIDs, nil
}

func checkMembersQuery(req ListMembersQuery) (bool, membersCursor, error) {
	if req.Limit < 1 || req.Limit > listMembersMax {
		return false, membersCursor{}, errs.New(errs.CodeInvalidInput, listMembersOp, slog.String("field", "limit"))
	}
	byShare := req.Order != OrderJoined
	if byShare && req.Order != OrderRoleShare && req.Order != "" {
		return false, membersCursor{}, errs.New(errs.CodeInvalidInput, listMembersOp, slog.String("field", "order"))
	}
	cursor, err := decodeMembersCursor(req.Cursor, byShare)
	return byShare, cursor, err
}

func monthRecords(
	ctx context.Context, reads MemberReads, cabalID ids.CabalID, userIDs []ids.UserID,
) (map[uuid.UUID]domain.MemberCalls, error) {
	now := reads.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	period := rankingport.Period{Kind: rankingport.PeriodMonth, Start: start, End: start.AddDate(0, 1, 0)}
	got, err := reads.Calls.Records(ctx, userIDs, &cabalID, period)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, listMembersOp)
	}
	out := make(map[uuid.UUID]domain.MemberCalls, len(got))
	for user, rec := range got {
		out[user.UUID()] = domain.MemberCalls{
			Right: rec.Right, Resolved: rec.Resolved, Voted: rec.Voted, Eligible: rec.Eligible,
		}
	}
	return out, nil
}

func memberKey(m domain.OrderedMember, byShare bool) domain.MemberKey {
	if byShare {
		return m.RoleKey()
	}
	return m.JoinedKey()
}

func pageBounds(members []domain.OrderedMember, cursor membersCursor, byShare bool, limit int) (int, int) {
	start := 0
	if cursor.ok {
		start = sort.Search(len(members), func(i int) bool {
			return domain.CompareMemberKeys(memberKey(members[i], byShare), cursor.key) > 0
		})
	}
	return start, min(start+limit, len(members))
}

type membersCursor struct {
	ok  bool
	key domain.MemberKey
}

func encodeMembersCursor(key domain.MemberKey, byShare bool) string {
	raw := strconv.FormatInt(key.Joined.UnixMicro(), 10) + "\x00" + key.UserID.String()
	if byShare {
		raw = strconv.Itoa(key.Rank) + "\x00" + strconv.FormatInt(int64(key.Bps), 10) + "\x00" + raw
	}
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeMembersCursor(raw string, byShare bool) (membersCursor, error) {
	if raw == "" {
		return membersCursor{}, nil
	}
	bad := func(err error) (membersCursor, error) {
		if err == nil {
			return membersCursor{}, errs.New(errs.CodeInvalidInput, listMembersOp, slog.String("field", "cursor"))
		}
		return membersCursor{}, errs.Wrap(err, errs.CodeInvalidInput, listMembersOp, slog.String("field", "cursor"))
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return bad(err)
	}
	parts := strings.Split(string(body), "\x00")
	want := 2
	if byShare {
		want = 4
	}
	if len(parts) != want {
		return bad(nil)
	}
	var key domain.MemberKey
	if byShare {
		if key.Rank, err = strconv.Atoi(parts[0]); err != nil {
			return bad(err)
		}
		bps, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil {
			return bad(err)
		}
		key.Bps, parts = int32(bps), parts[2:]
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return bad(err)
	}
	if key.UserID, err = uuid.Parse(parts[1]); err != nil {
		return bad(err)
	}
	key.Joined = time.UnixMicro(micros).UTC()
	return membersCursor{ok: true, key: key}, nil
}
