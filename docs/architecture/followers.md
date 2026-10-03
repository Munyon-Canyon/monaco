# Followers & following

**Status:** Decided 2026-09-26. Reconciled with [backend-platform.md](backend-platform.md) 2026-09-27: owned by the `social` module, [flow 20](backend-platform.md#flows), built in [Rollout](backend-platform.md#rollout) step 6.

## Decision

One table, `follows`: a user follows a user. Two user foreign keys, timestamps, and a `deleted_at` column. Unfollow is a soft delete (decided 2026-09-27). Follower and following counts are `count(*)` over live rows, backed by an index (decided 2026-09-27). There is no counts table and no recount job.

The `social` module owns the table ([Repository layout](backend-platform.md#repository-layout)). `users` belongs to `identity`, and a module never writes another module's table ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)), so the counts are not columns on `users`.

## Table

```sql
CREATE TABLE follows (
  id           uuid PRIMARY KEY,                      -- UUIDv7
  follower_id  uuid NOT NULL REFERENCES users (id),   -- the user doing the following
  followee_id  uuid NOT NULL REFERENCES users (id),   -- the user being followed
  source       text NOT NULL DEFAULT 'profile',       -- profile | phone | x | cabal | feed | suggested | referral
  created_at   timestamptz NOT NULL DEFAULT now(),
  deleted_at   timestamptz,                           -- null = live follow
  CHECK (follower_id <> followee_id)
);

-- One live follow per pair. A re-follow after an unfollow inserts a new row.
CREATE UNIQUE INDEX follows_pair_live_idx
  ON follows (follower_id, followee_id) WHERE deleted_at IS NULL;
-- "Who follows X" and the follower count: live followers of a user, newest first.
CREATE INDEX follows_followee_live_idx
  ON follows (followee_id, created_at DESC) WHERE deleted_at IS NULL;
-- "Who does X follow" and the following count.
CREATE INDEX follows_follower_live_idx
  ON follows (follower_id, created_at DESC) WHERE deleted_at IS NULL;
```

Notes:

- **Both sides are indexed.** Listing someone's followers reads by `followee_id`; listing who they follow reads by `follower_id`. An index on only one user id makes the other list a full scan.
- **One live row per pair.** The unique partial index means a user cannot follow someone twice at once. Unfollow sets `deleted_at` on the live row. A re-follow inserts a new row, so the table keeps each follow cycle. Every read filters `deleted_at IS NULL`.
- `source` records where the follow came from, for the social-graph analytics in [analytics-admin.md](analytics-admin.md#business-metrics) and the suggestions in [auth.md](auth.md#social-graph). `referral` follows are written by the `social` consumer of `referral.attributed` ([referrals.md](referrals.md#attaching-a-referral)), not by the `referrals` module.

## Writes

**Follow** `POST /v1/users/{id}/follow` runs the `Follow` command in one `uow.Do` ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). It takes an `Idempotency-Key` like every mutating call ([Thin client](backend-platform.md#thin-client)), though the SQL below is idempotent without it:

```sql
INSERT INTO follows (id, follower_id, followee_id, source) VALUES ($id, $me, $them, $source)
ON CONFLICT (follower_id, followee_id) WHERE deleted_at IS NULL DO NOTHING
RETURNING id;
```

If a row came back (a new follow or a re-follow), append the `events` row `follow.created`. Bus consumers do the rest: `notify` (see [Notifications](#notifications)) and `analytics`, per flow 20. The feed with `scope=following` reads `follows` directly in the `social` module, so `social` has no `follow.*` consumer. If no row came back, the follow was already active: return success and change nothing. Retries and double taps are therefore idempotent and can never double-count.

**Unfollow** `DELETE /v1/users/{id}/follow` runs the `Unfollow` command: `UPDATE follows SET deleted_at = now() WHERE follower_id = $me AND followee_id = $them AND deleted_at IS NULL`. Only if one row was updated, append `follow.removed`.

Timestamps come from the injected `clock.Clock`, not `now()` in handler code ([Lint](backend-platform.md#lint)); the SQL above shows the shape.

**Rules:** no self-follow (DB check). Cannot follow a `BANNED` or `DELETED` user. `social` learns a user's status from the `identity` module's read-only query port. Each refusal is an `errs` code and an outcome on flow 20 ([Errors](backend-platform.md#errors)). When a user is banned or deleted, their follows stay in the table but are excluded from lists and their counts are not shown.

## Scope

Defaults 2026-09-27:

- **Users only.** There are no cabal follows in MVP. If they come, they get a separate `cabal_follows` table with the same shape, not a polymorphic column here.
- **No private accounts.** Follows are instant; there are no follow requests.
- **Blocking ships with comment moderation** ([feed.md](feed.md#comments)). A block removes the follow both ways and hides the blocked user's content. It is built in the same wave as reporting and admin removal, not before.

## Notifications

One rule, reconciling the two notification defaults of 2026-09-27. A new follower sends one push ("X followed you") until the followee hits a daily cap. Follows past the cap batch into one push a day ("5 people followed you"). The cap value is a `notify` tuning knob ([notifications.md](notifications.md)).

Unfollows never notify. `follow.removed` only feeds analytics.

## Reads

| Route | Query |
| --- | --- |
| `GET /v1/users/{id}/followers` | Active rows by `followee_id`, keyset paginated on `(created_at, follower_id)`. |
| `GET /v1/users/{id}/following` | Active rows by `follower_id`, same pagination. |
| `GET /v1/users/{id}` | Includes `follower_count`, `following_count`, and `followed_by_me`. |

Each count is `count(*) FROM follows WHERE followee_id = $id AND deleted_at IS NULL`, or the same on `follower_id`. The two partial indexes above serve them. With no stored count there is nothing to drift and no recount job. The profile route belongs to `identity`, which gets the counts and `followed_by_me` from the `social` query port, so the screen is still one `GET`.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Hard delete on unfollow | Loses state; the product wants to know who unfollowed. |
| One row per pair, re-follow clears the delete column | Loses earlier follow cycles, and a soft delete that can be undone is not the shared pattern for `users` and chat. |
| Denormalized `follow_counts` table | Earlier draft. Needs a counter update in every write, a lock order, and a nightly recount. An indexed `count(*)` is fast enough and cannot drift. |
| Postgres trigger to maintain counts | Hides a write inside the schema, and still stores a count that can drift. |
| Counts as columns on `users` | Earlier draft. `users` belongs to `identity`; writing it from `social` breaks the module wall. |

## Open questions

None at the moment.

Log: [log/followers.md](log/followers.md).
