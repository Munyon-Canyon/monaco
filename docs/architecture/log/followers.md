# Followers & following log

Dated record of changes to [followers.md](../followers.md). Add one line per change, newest last.

- 2026-09-27: Decided: unfollow is a soft delete through `deleted_at`, and a unique partial index on `(follower_id, followee_id) WHERE deleted_at IS NULL` lets a re-follow insert a new row. Dropped `follow_counts`; counts are an indexed `count(*)` over live rows. Removed the nightly recount job and the lock-order rule.
- 2026-09-27: Defaults applied: users only, no cabal follows in MVP; no private accounts; blocking ships with comment moderation; a push per new follower up to a daily cap, then one batched push a day (reconciles the "batch follows" and "push per follow with cap" defaults); no unfollow notification. `analytics` added as a flow 20 consumer. All open questions closed.
- 2026-09-27: Reconciled with [backend-platform.md](../backend-platform.md). Owner is the `social` module, flow 20, rollout step 6. Counts move from `users` columns to a `social`-owned `follow_counts` table, read by `identity` through a query port. Events renamed to `follow.created` / `follow.removed`; unfollow now appends `follow.removed`. `Follow` and `Unfollow` are commands with `Idempotency-Key`. Status checks use the `identity` query port. Referral auto-follows come from a `social` consumer. Nightly count job takes an advisory lock.
- 2026-09-26: Outbox rows replaced by `events` rows delivered over the NATS event bus ([event-bus.md](../event-bus.md)).
- 2026-09-26: Initial decision. Single `follows` table, soft unfollow, denormalized counts on users.
- 2026-10-02: Decided: `social` has no `follow.*` consumer for the MVP. The feed reads `follows` directly, so the flow 20 consumers are `notify` and `analytics`.
