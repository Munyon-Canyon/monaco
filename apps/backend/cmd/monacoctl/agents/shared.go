package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	recordMarker = "<!-- monacoctl agents record -->"
	jsonFence    = "```json\n"
)

type sharedRecord struct {
	Ticket  int       `json:"ticket"`
	Model   string    `json:"model"`
	Branch  string    `json:"branch"`
	Base    string    `json:"base"`
	State   State     `json:"state"`
	Started time.Time `json:"started"`
	Changed time.Time `json:"changed"`
	Queued  *Queue    `json:"queued"`
}

func recordBody(r Record) string {
	raw, _ := json.Marshal(sharedRecord{
		Ticket: r.Ticket, Model: r.Model, Branch: r.Branch, Base: r.Base,
		State: r.State, Started: r.Started, Changed: r.Changed, Queued: r.Queued,
	})
	branch := r.Branch
	if branch == "" {
		branch = "none yet"
	}
	return fmt.Sprintf(
		"%s\nOwner record for #%d: model %s, state %s, branch %s, parent %.8s, started %s.\n%s%s\n```\n",
		recordMarker, r.Ticket, r.Model, r.State, branch, r.Base,
		r.Started.UTC().Format(time.RFC3339), jsonFence, raw,
	)
}

func newestTrustedRecord(all []Comment) (Comment, bool) {
	for _, c := range slices.Backward(all) {
		if c.trusted() && strings.Contains(c.Body, recordMarker) {
			return c, true
		}
	}
	return Comment{}, false
}

func (env *Env) rebuildRecord(ctx context.Context, ticket int) (Record, error) {
	all, err := env.issueComments(ctx, ticket)
	if err != nil {
		return Record{}, err
	}
	c, ok := newestTrustedRecord(all)
	if !ok {
		return Record{}, noRecord(ticket)
	}
	_, rest, _ := strings.Cut(c.Body, jsonFence)
	raw, _, _ := strings.Cut(rest, "\n```")
	var s sharedRecord
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Record{}, detailErr(
			errs.CodeDecodeFailed,
			"monacoctl.agents.record",
			fmt.Sprintf("the owner record comment on #%d does not decode: %v", ticket, err),
		)
	}
	r := Record{
		Ticket: ticket, Model: s.Model, Worktree: env.worktreePath(ticket), Branch: s.Branch, Base: s.Base,
		State: s.State, Queued: s.Queued, Started: s.Started, Changed: s.Changed,
	}
	return r, env.saveRecord(r)
}

func (env *Env) publishRecord(ctx context.Context, r Record) error {
	all, err := env.issueComments(ctx, r.Ticket)
	if err != nil {
		return err
	}
	c, ok := newestTrustedRecord(all)
	if ok {
		me, err := env.Run(ctx, env.Work, "", "gh", "api", "user", "--jq", ".login")
		if err != nil {
			return err
		}
		ok = c.User.Login == strings.TrimSpace(string(me))
	}
	return env.writeComment(ctx, r.Ticket, c.ID, ok, recordBody(r))
}

func (env *Env) storeRecord(ctx context.Context, r Record) error {
	if err := env.saveRecord(r); err != nil {
		return err
	}
	return env.publishRecord(ctx, r)
}
