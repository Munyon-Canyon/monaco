package agents

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	contextList = "pageInfo{hasNextPage endCursor} nodes{" +
		"... on CheckRun{name status conclusion completedAt databaseId detailsUrl} " +
		"... on StatusContext{context state createdAt}}"
	commitChecks = "oid statusCheckRollup{contexts(first:100){" + contextList + "}}"
)

type gqlCommit struct {
	OID               string     `json:"oid"`
	CommittedDate     time.Time  `json:"committedDate"`
	StatusCheckRollup *gqlRollup `json:"statusCheckRollup"`
}

type gqlRollup struct {
	Contexts struct {
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
		Nodes []gqlContext `json:"nodes"`
	} `json:"contexts"`
}

type gqlContext struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	CompletedAt time.Time `json:"completedAt"`
	DatabaseID  int64     `json:"databaseId"`
	DetailsURL  string    `json:"detailsUrl"`
	Context     string    `json:"context"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (c gqlCommit) latest() []gqlContext {
	if c.StatusCheckRollup == nil {
		return nil
	}
	at := map[string]int{}
	var out []gqlContext
	for _, x := range c.StatusCheckRollup.Contexts.Nodes {
		key := x.Name + "\x00" + x.Context
		i, seen := at[key]
		switch {
		case !seen:
			at[key] = len(out)
			out = append(out, x)
		case x.newerThan(out[i]):
			out[i] = x
		}
	}
	return out
}

func (x gqlContext) newerThan(y gqlContext) bool {
	a, b := x.CompletedAt, y.CompletedAt
	if x.Context != "" {
		a, b = x.CreatedAt, y.CreatedAt
	}
	return a.IsZero() || (!b.IsZero() && a.After(b))
}

func readAllChecks(ctx context.Context, query func(context.Context, string, any) error, commits []*gqlCommit) error {
	for {
		q, more := nextChecks(commits)
		if len(more) == 0 {
			return nil
		}
		var data struct {
			Repository map[string]*gqlCommit `json:"repository"`
		}
		if err := query(ctx, q, &data); err != nil {
			return err
		}
		for i, c := range more {
			next := data.Repository["c"+strconv.Itoa(i)]
			if next == nil || next.StatusCheckRollup == nil {
				return detailErr(errs.CodeUpstreamUnavailable, "monacoctl.agents.graphql",
					fmt.Sprintf("commit %s lost its checks while they were paged", c.OID))
			}
			rest := next.StatusCheckRollup.Contexts
			was := c.StatusCheckRollup.Contexts.PageInfo.EndCursor
			if rest.PageInfo.HasNextPage && len(rest.Nodes) == 0 && rest.PageInfo.EndCursor == was {
				return detailErr(errs.CodeUpstreamUnavailable, "monacoctl.agents.graphql",
					fmt.Sprintf("commit %s returned an empty page of checks without moving past cursor %s", c.OID, was))
			}
			c.StatusCheckRollup.Contexts.Nodes = append(c.StatusCheckRollup.Contexts.Nodes, rest.Nodes...)
			c.StatusCheckRollup.Contexts.PageInfo = rest.PageInfo
		}
	}
}

func nextChecks(commits []*gqlCommit) (query string, more []*gqlCommit) {
	var b strings.Builder
	b.WriteString(repoQuery)
	for _, c := range commits {
		if c.StatusCheckRollup == nil || !c.StatusCheckRollup.Contexts.PageInfo.HasNextPage {
			continue
		}
		_, _ = fmt.Fprintf(&b,
			"c%d: object(oid:%q){... on Commit{statusCheckRollup{contexts(first:100,after:%q){%s}}}} ",
			len(more), c.OID, c.StatusCheckRollup.Contexts.PageInfo.EndCursor, contextList)
		more = append(more, c)
	}
	b.WriteString("}}")
	return b.String(), more
}
