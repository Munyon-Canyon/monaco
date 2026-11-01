package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	socialport "github.com/monaco/monaco/apps/backend/internal/modules/social/port"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const opCatchUp = "notify.CatchUp"

type ReplyCounts interface {
	ReplyCounts(ctx context.Context, proposalIDs []ids.ProposalID) (map[ids.ProposalID]int, error)
}

type AssetsByMint interface {
	ByMints(ctx context.Context, mints []string) ([]market.Asset, error)
}

type CatchUpCabals interface {
	CabalsOf(ctx context.Context, user ids.UserID) ([]ids.CabalID, error)
	Cabals(ctx context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error)
}

type CatchUpPorts struct {
	Governance governanceport.CatchUp
	Comments   socialport.CommentPreviews
	Replies    ReplyCounts
	Invites    cabalport.InviteReader
	InviteByID cabalport.InviteLookup
	Cabals     CatchUpCabals
	Holdings   treasuryport.TopHoldingsReader
	Assets     AssetsByMint
	Users      Users
	Reads      sqlc.DBTX
	Clock      clock.Clock
}

type ReadCatchUp struct{ ports CatchUpPorts }

func NewReadCatchUp(ports CatchUpPorts) *ReadCatchUp { return &ReadCatchUp{ports: ports} }

type itemState struct {
	readAt, clearedAt *time.Time
}

type sourceRow struct {
	key       string
	kind      domain.CatchUpKind
	createdAt time.Time
	data      map[string]string
}

type candidate struct {
	key     string
	kind    domain.CatchUpKind
	at      time.Time
	unread  bool
	pending *governanceport.CatchUpProposal
	invite  *cabalport.PendingInvite
	row     *sourceRow
	comment *socialport.CommentPreview
}

type catchUpSet struct {
	now        time.Time
	actives    []candidate
	cards      map[ids.ProposalID]governanceport.CatchUpProposal
	previews   map[uuid.UUID]socialport.CommentPreview
	activeKeys map[string]bool
	joined     []ids.CabalID
}

func (r *ReadCatchUp) Count(ctx context.Context, user ids.UserID) (int, error) {
	set, err := r.collect(ctx, user)
	if err != nil {
		return 0, err
	}
	return len(set.actives), nil
}

func (r *ReadCatchUp) Read(ctx context.Context, user ids.UserID, since time.Time) (domain.CatchUp, error) {
	set, err := r.collect(ctx, user)
	if err != nil {
		return domain.CatchUp{}, err
	}
	deco, err := r.decorate(ctx, set)
	if err != nil {
		return domain.CatchUp{}, err
	}
	out := domain.CatchUp{
		Count: len(set.actives), Cabals: deco.cabalList,
		Waiting: []domain.CatchUpItem{}, Replies: []domain.CatchUpItem{},
		Invites: []domain.CatchUpItem{}, Results: []domain.CatchUpItem{},
	}
	for _, c := range set.actives {
		item := deco.item(c, user)
		switch c.kind {
		case domain.CatchUpWaitingVote:
			out.Waiting = append(out.Waiting, item)
		case domain.CatchUpReply, domain.CatchUpMention:
			out.Replies = append(out.Replies, item)
		case domain.CatchUpInvite:
			out.Invites = append(out.Invites, item)
		case domain.CatchUpResult:
			out.Results = append(out.Results, item)
		}
	}
	if out.Cleared, err = r.cleared(ctx, user, since, set.activeKeys); err != nil {
		return domain.CatchUp{}, err
	}
	return out, nil
}

type sources struct {
	pending  []governanceport.CatchUpProposal
	rows     []sourceRow
	invites  []cabalport.PendingInvite
	states   map[string]itemState
	previews map[uuid.UUID]socialport.CommentPreview
	cards    map[ids.ProposalID]governanceport.CatchUpProposal
	joined   []ids.CabalID
	member   map[string]bool
}

func (r *ReadCatchUp) collect(ctx context.Context, user ids.UserID) (catchUpSet, error) {
	now := r.ports.Clock.Now()
	src, err := r.gather(ctx, user, now)
	if err != nil {
		return catchUpSet{}, err
	}
	return src.activeSet(user, now), nil
}

func (r *ReadCatchUp) gather(ctx context.Context, user ids.UserID, now time.Time) (sources, error) {
	var src sources
	var err error
	if src.pending, err = r.ports.Governance.PendingVotes(ctx, user); err != nil {
		return sources{}, err
	}
	if src.rows, err = r.sourceRows(ctx, user, now); err != nil {
		return sources{}, err
	}
	if src.invites, err = r.ports.Invites.PendingInvites(ctx, user, now); err != nil {
		return sources{}, err
	}
	if src.states, err = r.states(ctx, user, src.pending, src.rows, src.invites); err != nil {
		return sources{}, err
	}
	if src.previews, err = r.previews(ctx, user, src.rows); err != nil {
		return sources{}, err
	}
	if src.cards, err = r.cards(ctx, user, src.rows, src.previews); err != nil {
		return sources{}, err
	}
	if src.joined, err = r.ports.Cabals.CabalsOf(ctx, user); err != nil {
		return sources{}, err
	}
	src.member = make(map[string]bool, len(src.joined))
	for _, id := range src.joined {
		src.member[id.String()] = true
	}
	return src, nil
}

func (s sources) activeSet(user ids.UserID, now time.Time) catchUpSet {
	set := catchUpSet{
		now: now, cards: s.cards, previews: s.previews, activeKeys: map[string]bool{}, joined: s.joined,
	}
	for i := range s.pending {
		p := s.pending[i]
		set.add(s.states, candidate{
			key: domain.ItemKeyProposalPrefix + p.ID.String(), kind: domain.CatchUpWaitingVote, at: p.CreatedAt,
			pending: &p,
		}, false)
	}
	for i := range s.rows {
		if c, decided, ok := s.rowCandidate(user, s.rows[i]); ok && !set.activeKeys[c.key] {
			set.add(s.states, c, decided)
		}
	}
	for i := range s.invites {
		inv := s.invites[i]
		set.add(s.states, candidate{
			key: domain.ItemKeyInvitePrefix + inv.ID.String(), kind: domain.CatchUpInvite, at: inv.CreatedAt,
			invite: &inv,
		}, false)
	}
	return set
}

func (s sources) rowCandidate(user ids.UserID, row sourceRow) (candidate, bool, bool) {
	c := candidate{key: row.key, kind: row.kind, at: row.createdAt, row: &row}
	switch {
	case row.kind == domain.CatchUpResult:
		card, ok := s.cards[proposalOf(row)]
		return c, card.DecidedBy != nil && *card.DecidedBy == user, ok
	case strings.HasPrefix(row.key, domain.ItemKeyCommentPrefix):
		preview, ok := s.previews[commentOf(row)]
		c.comment = &preview
		return c, false, ok
	}
	return c, false, s.member[row.data["cabal_id"]]
}

func (set *catchUpSet) add(states map[string]itemState, c candidate, decided bool) {
	st := states[c.key]
	if !isActive(st, c.at, decided) {
		return
	}
	c.unread = st.readAt == nil || c.at.After(*st.readAt)
	set.actives = append(set.actives, c)
	set.activeKeys[c.key] = true
}

func isActive(st itemState, newest time.Time, decidedByViewer bool) bool {
	if st.clearedAt == nil {
		return true
	}
	return newest.After(*st.clearedAt) && !decidedByViewer
}

func (r *ReadCatchUp) sourceRows(ctx context.Context, user ids.UserID, now time.Time) ([]sourceRow, error) {
	got, err := sqlc.New(r.ports.Reads).CatchUpRows(ctx, sqlc.CatchUpRowsParams{
		UserID: user.UUID(), Kinds: domain.CatchUpNotificationKinds(), Since: now.Add(-domain.CatchUpWindow),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, opCatchUp)
	}
	seen := map[string]bool{}
	rows := make([]sourceRow, 0, len(got))
	for _, g := range got {
		kind, ok := domain.CatchUpKindOf(g.Kind)
		if !ok || !g.ItemKey.Valid || seen[g.ItemKey.String] {
			continue
		}
		var data map[string]string
		if err := json.Unmarshal(g.Data, &data); err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, opCatchUp)
		}
		seen[g.ItemKey.String] = true
		rows = append(rows, sourceRow{key: g.ItemKey.String, kind: kind, createdAt: g.CreatedAt, data: data})
	}
	return rows, nil
}

func (r *ReadCatchUp) states(
	ctx context.Context,
	user ids.UserID,
	pending []governanceport.CatchUpProposal,
	rows []sourceRow,
	invites []cabalport.PendingInvite,
) (map[string]itemState, error) {
	keys := make([]string, 0, len(pending)+len(rows)+len(invites))
	for _, p := range pending {
		keys = append(keys, domain.ItemKeyProposalPrefix+p.ID.String())
	}
	for _, row := range rows {
		keys = append(keys, row.key)
	}
	for _, inv := range invites {
		keys = append(keys, domain.ItemKeyInvitePrefix+inv.ID.String())
	}
	states := make(map[string]itemState, len(keys))
	if len(keys) == 0 {
		return states, nil
	}
	got, err := sqlc.New(r.ports.Reads).CatchUpState(ctx, sqlc.CatchUpStateParams{UserID: user.UUID(), Keys: keys})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, opCatchUp)
	}
	for _, g := range got {
		st := itemState{}
		if g.ReadAt.Valid {
			st.readAt = &g.ReadAt.Time
		}
		if g.ClearedAt.Valid {
			st.clearedAt = &g.ClearedAt.Time
		}
		states[g.ItemKey] = st
	}
	return states, nil
}

func commentOf(row sourceRow) uuid.UUID {
	id, _ := domain.ItemKeyID(row.key, domain.ItemKeyCommentPrefix)
	parsed, _ := uuid.Parse(id)
	return parsed
}

func proposalOf(row sourceRow) ids.ProposalID {
	id, _ := domain.ItemKeyID(row.key, domain.ItemKeyProposalPrefix)
	parsed, _ := uuid.Parse(id)
	return ids.ProposalIDFrom(parsed)
}

func (r *ReadCatchUp) previews(
	ctx context.Context, user ids.UserID, rows []sourceRow,
) (map[uuid.UUID]socialport.CommentPreview, error) {
	var comments []uuid.UUID
	for _, row := range rows {
		if strings.HasPrefix(row.key, domain.ItemKeyCommentPrefix) {
			comments = append(comments, commentOf(row))
		}
	}
	if len(comments) == 0 {
		return map[uuid.UUID]socialport.CommentPreview{}, nil
	}
	return r.ports.Comments.CommentPreviews(ctx, user, comments)
}

func distinctProposals(in []ids.ProposalID) []ids.ProposalID {
	seen := make(map[ids.ProposalID]bool, len(in))
	out := make([]ids.ProposalID, 0, len(in))
	for _, id := range in {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (r *ReadCatchUp) cards(
	ctx context.Context, user ids.UserID, rows []sourceRow, previews map[uuid.UUID]socialport.CommentPreview,
) (map[ids.ProposalID]governanceport.CatchUpProposal, error) {
	var proposals []ids.ProposalID
	for _, row := range rows {
		switch {
		case row.kind == domain.CatchUpResult:
			proposals = append(proposals, proposalOf(row))
		case strings.HasPrefix(row.key, domain.ItemKeyCommentPrefix):
			if p, ok := previews[commentOf(row)]; ok && p.ProposalID != nil {
				proposals = append(proposals, *p.ProposalID)
			}
		}
	}
	if len(proposals) == 0 {
		return map[ids.ProposalID]governanceport.CatchUpProposal{}, nil
	}
	return r.ports.Governance.ProposalCards(ctx, user, distinctProposals(proposals))
}

type decoration struct {
	cabals    map[string]domain.CatchUpCabal
	cabalList []domain.CatchUpCabal
	replies   map[ids.ProposalID]int
	holdings  map[ids.CabalID]string
	authors   map[ids.UserID]domain.CatchUpPerson
	set       catchUpSet
}

func (r *ReadCatchUp) decorate(ctx context.Context, set catchUpSet) (decoration, error) {
	deco := decoration{set: set}
	var err error
	if err = r.decorateCabals(ctx, &deco); err != nil {
		return decoration{}, err
	}
	if err = r.decorateReplies(ctx, &deco); err != nil {
		return decoration{}, err
	}
	if err = r.decorateHoldings(ctx, &deco); err != nil {
		return decoration{}, err
	}
	if err = r.decorateAuthors(ctx, &deco); err != nil {
		return decoration{}, err
	}
	return deco, nil
}

func (r *ReadCatchUp) decorateCabals(ctx context.Context, deco *decoration) error {
	joined := deco.set.joined
	deco.cabals = map[string]domain.CatchUpCabal{}
	deco.cabalList = []domain.CatchUpCabal{}
	if len(joined) == 0 {
		return nil
	}
	views, err := r.ports.Cabals.Cabals(ctx, joined)
	if err != nil {
		return err
	}
	for _, id := range joined {
		v, ok := views[id]
		if !ok {
			continue
		}
		c := domain.CatchUpCabal{ID: id.String(), Name: v.Name, PictureURL: v.PictureURL}
		deco.cabals[c.ID] = c
		deco.cabalList = append(deco.cabalList, c)
	}
	return nil
}

func (r *ReadCatchUp) decorateReplies(ctx context.Context, deco *decoration) error {
	seen := map[ids.ProposalID]bool{}
	var proposals []ids.ProposalID
	note := func(id ids.ProposalID) {
		if !seen[id] {
			seen[id] = true
			proposals = append(proposals, id)
		}
	}
	for _, c := range deco.set.actives {
		if c.pending != nil {
			note(c.pending.ID)
		}
	}
	for id := range deco.set.cards {
		note(id)
	}
	if len(proposals) == 0 {
		deco.replies = map[ids.ProposalID]int{}
		return nil
	}
	var err error
	deco.replies, err = r.ports.Replies.ReplyCounts(ctx, proposals)
	return err
}

func (r *ReadCatchUp) decorateHoldings(ctx context.Context, deco *decoration) error {
	deco.holdings = map[ids.CabalID]string{}
	var cabals []ids.CabalID
	for _, c := range deco.set.actives {
		if c.invite != nil {
			cabals = append(cabals, c.invite.CabalID)
		}
	}
	if len(cabals) == 0 {
		return nil
	}
	top, err := r.ports.Holdings.TopHoldings(ctx, cabals)
	if err != nil {
		return err
	}
	mints := make([]string, 0, len(top))
	for _, m := range top {
		mints = append(mints, string(m))
	}
	if len(mints) == 0 {
		return nil
	}
	assets, err := r.ports.Assets.ByMints(ctx, mints)
	if err != nil {
		return err
	}
	names := make(map[string]string, len(assets))
	for _, a := range assets {
		names[a.Mint.String()] = nameOrSymbol(a)
	}
	for cabal, mint := range top {
		if name, ok := names[string(mint)]; ok {
			deco.holdings[cabal] = name
		}
	}
	return nil
}

func nameOrSymbol(a market.Asset) string {
	if a.DisplayName != "" {
		return a.DisplayName
	}
	return a.Symbol
}

func (r *ReadCatchUp) decorateAuthors(ctx context.Context, deco *decoration) error {
	deco.authors = map[ids.UserID]domain.CatchUpPerson{}
	var authors []ids.UserID
	for _, c := range deco.set.actives {
		if c.row == nil || !strings.HasPrefix(c.key, domain.ItemKeyChatPrefix) {
			continue
		}
		if id, err := ids.ParseUserID(c.row.data["author_id"]); err == nil {
			authors = append(authors, id)
		}
	}
	if len(authors) == 0 {
		return nil
	}
	cards, err := r.ports.Users.UsersByID(ctx, slices.Compact(slices.SortedFunc(slices.Values(authors), compareUsers)))
	if err != nil {
		return err
	}
	for id, card := range cards {
		if !card.Deleted {
			deco.authors[id] = domain.CatchUpPerson{ID: id.String(), Name: card.DisplayName, PhotoURL: card.PhotoURL}
		}
	}
	return nil
}

func compareUsers(a, b ids.UserID) int { return strings.Compare(a.String(), b.String()) }

func (d decoration) cabalRef(id string, fallbackName string) *domain.CatchUpCabal {
	if c, ok := d.cabals[id]; ok {
		return &c
	}
	if id == "" || fallbackName == "" {
		return nil
	}
	return &domain.CatchUpCabal{ID: id, Name: fallbackName}
}

func (d decoration) proposal(card governanceport.CatchUpProposal) *domain.CatchUpProposal {
	out := &domain.CatchUpProposal{
		ID: card.ID.String(), Kind: card.Kind, Symbol: card.Symbol, AssetName: card.AssetName,
		AssetLogoURL: card.AssetLogo, USDCMicros: card.USDCMicros, TokenAmount: card.TokenAmount,
		Proposer: domain.CatchUpPerson{
			ID: card.Proposer.ID.String(), Name: card.Proposer.Name, PhotoURL: card.Proposer.PhotoURL,
		},
		CreatedAt: card.CreatedAt, ExpiresAt: card.ExpiresAt, Status: string(card.Status),
		Yes: card.Yes, No: card.No, Voted: card.Voted, Eligible: card.Eligible,
		ReplyCount: d.replies[card.ID], YourChoice: card.YourChoice,
	}
	return out
}

func (d decoration) item(c candidate, viewer ids.UserID) domain.CatchUpItem {
	item := domain.CatchUpItem{Key: c.key, Kind: c.kind, Unread: c.unread, At: c.at}
	switch {
	case c.pending != nil:
		item.Proposal = d.proposal(*c.pending)
		item.Cabal = d.cabalRef(c.pending.CabalID.String(), c.pending.CabalName)
	case c.invite != nil:
		d.inviteItem(&item, *c.invite)
	case c.kind == domain.CatchUpResult:
		d.resultItem(&item, *c.row)
	case c.comment != nil:
		d.commentItem(&item, c, viewer)
	default:
		d.chatItem(&item, *c.row)
	}
	return item
}

func (d decoration) inviteItem(item *domain.CatchUpItem, inv cabalport.PendingInvite) {
	item.Actor = &domain.CatchUpPerson{ID: inv.InviterID.String(), Name: inv.InviterName, PhotoURL: inv.InviterPhotoURL}
	item.Cabal = &domain.CatchUpCabal{ID: inv.CabalID.String(), Name: inv.CabalName, PictureURL: inv.PictureURL}
	item.Invite = &domain.CatchUpInviteInfo{
		ID: inv.ID.String(), MemberCount: inv.MemberCount, TopHoldingName: d.holdings[inv.CabalID],
	}
}

func (d decoration) resultItem(item *domain.CatchUpItem, row sourceRow) {
	card := d.set.cards[proposalOf(row)]
	p := d.proposal(card)
	p.Outcome = row.data["outcome"]
	filled := p.Outcome == "filled"
	p.Filled = &filled
	item.Proposal = p
	item.Cabal = d.cabalRef(card.CabalID.String(), card.CabalName)
}

func (d decoration) commentItem(item *domain.CatchUpItem, c candidate, viewer ids.UserID) {
	preview := *c.comment
	item.Actor = &domain.CatchUpPerson{
		ID: preview.Author.ID.String(), Name: preview.Author.Name, PhotoURL: preview.Author.PhotoURL,
	}
	item.Comment = commentView(preview)
	if preview.ProposalID != nil {
		if card, ok := d.set.cards[*preview.ProposalID]; ok {
			item.Proposal = d.proposal(card)
			item.Cabal = d.cabalRef(card.CabalID.String(), card.CabalName)
		}
	} else {
		item.Post = &domain.CatchUpPost{
			FeedItemID: preview.FeedItemID.String(), Kind: c.row.data["item_kind"], Label: c.row.data["item_label"],
			ActorName: c.row.data["item_actor_name"],
		}
	}
	if item.Cabal == nil && preview.CabalID != nil {
		item.Cabal = d.cabalRef(preview.CabalID.String(), "")
	}
	item.Quote = d.quote(c, preview, viewer)
}

func commentView(p socialport.CommentPreview) *domain.CatchUpComment {
	out := &domain.CatchUpComment{
		FeedItemID: p.FeedItemID.String(), CommentID: p.CommentID.String(), RootCommentID: p.RootCommentID.String(),
		Author: domain.CatchUpPerson{ID: p.Author.ID.String(), Name: p.Author.Name, PhotoURL: p.Author.PhotoURL},
		Body:   p.Body, CreatedAt: p.CreatedAt, ReactionCount: p.ReactionCount,
	}
	for _, r := range p.Reactions {
		out.Reactions = append(out.Reactions, domain.CatchUpReaction{Emoji: r.Emoji, Count: r.Count, Mine: r.Mine})
	}
	if p.ParentCommentID != nil {
		out.ParentCommentID = p.ParentCommentID.String()
	}
	if p.ProposalID != nil {
		out.ProposalID = p.ProposalID.String()
	}
	if p.CabalID != nil {
		out.CabalID = p.CabalID.String()
	}
	return out
}

func (d decoration) quote(c candidate, preview socialport.CommentPreview, viewer ids.UserID) *domain.CatchUpQuote {
	if c.kind == domain.CatchUpMention {
		return &domain.CatchUpQuote{AuthorName: preview.Author.Name, Text: preview.Body}
	}
	if preview.ReplyToID == nil {
		return nil
	}
	parent, ok := d.set.previews[*preview.ReplyToID]
	if !ok {
		return nil
	}
	return &domain.CatchUpQuote{AuthorName: parent.Author.Name, IsYou: parent.Author.ID == viewer, Text: parent.Body}
}

func (d decoration) chatItem(item *domain.CatchUpItem, row sourceRow) {
	if id, err := ids.ParseUserID(row.data["author_id"]); err == nil {
		if p, ok := d.authors[id]; ok {
			item.Actor = &p
		}
	}
	item.Cabal = d.cabalRef(row.data["cabal_id"], "")
}

func (r *ReadCatchUp) cleared(
	ctx context.Context, user ids.UserID, since time.Time, active map[string]bool,
) ([]domain.CatchUpItem, error) {
	got, err := sqlc.New(r.ports.Reads).ClearedSince(ctx, sqlc.ClearedSinceParams{UserID: user.UUID(), Since: since})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, opCatchUp)
	}
	out := make([]domain.CatchUpItem, 0, len(got))
	for _, g := range got {
		if active[g.ItemKey] {
			continue
		}
		var item domain.CatchUpItem
		if len(g.Snapshot) == 0 || json.Unmarshal(g.Snapshot, &item) != nil {
			observability.Degraded(ctx, observability.NotifyCatchUpSnapshotMissing,
				slog.String("user_id", user.String()), slog.String("item_key", g.ItemKey))
			continue
		}
		item.Key, item.Unread, item.At = g.ItemKey, false, g.ClearedAt.Time
		item.ClearedReason = g.ClearedReason.String
		out = append(out, item)
	}
	return out, nil
}

func (r *ReadCatchUp) Render(
	ctx context.Context, user ids.UserID, keys []string,
) (map[string]domain.CatchUpItem, error) {
	set, err := r.collect(ctx, user)
	if err != nil {
		return nil, err
	}
	set.actives = slices.DeleteFunc(set.actives, func(c candidate) bool { return !slices.Contains(keys, c.key) })
	deco, err := r.decorate(ctx, set)
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.CatchUpItem, len(set.actives))
	for _, c := range set.actives {
		out[c.key] = deco.item(c, user)
	}
	return out, nil
}
