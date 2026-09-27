# Data model

**Status:** Decided 2026-09-27. Table list decided 2026-09-26. Naming, IDs, money types, migrations and module ownership follow [backend-platform.md](backend-platform.md). The remaining questions were settled on 2026-09-27, by the user or as reversible defaults; see [Log](#log).

## Decision

The planned table set is below. Names are the conceptual names from the design discussion. In Postgres they are plural `snake_case` with `cabal` in place of `group` (e.g. `cabals`, `cabal_members`, `cabal_txns`); see [Naming](#naming).

The rewrite starts on an empty database. No user data moves from the old backend: every row is wiped at cutover (decided 2026-09-27). The **Replaces today's** column is lineage only, so a reader can find the old table. Nothing is migrated or backfilled from it. Privy wallets live on-chain and in Privy, not in Monaco's database, so a returning user keeps their wallet; see [UserWallets](#wallets).

Each table belongs to exactly one module of the [backend rewrite](backend-platform.md#repository-layout). Only the owning module writes it. Other modules react to its events or read through a query port the owner exports ([dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)). The **Module** column records that owner.

| Planned table | Module | Purpose | Replaces today's |
| --- | --- | --- | --- |
| **Users** | identity | One row per person. Profile, `handle` (unique username picked in onboarding; decided 2026-09-27), photo, `auth_state` (onboarding progress) and `account_status` (`active`, `suspended`, `banned`, `deleted`; default 2026-09-27). `first_deposit_at` is set by `identity`'s consumer of `deposit.credited` on the first deposit of at least $10. Soft delete through `deleted_at` (decided 2026-09-27). Full columns in [auth.md](auth.md). | `users` |
| **UserWallets** | identity | One Privy wallet per user: Privy wallet id, address. | `member_wallets` |
| **Cabals** | cabal | One row per cabal. Name, picture, rules (threshold, slippage), creator, `status` (`active` / `banned`). Every cabal is public (decided 2026-09-27), so there is no visibility column. No pause columns; the pause lives in `funding` (below). | `groups` |
| **TreasuryWallets** | cabal | One app-owned Privy treasury wallet per cabal: Privy wallet id, address. | `treasuries` |
| **CabalMembers** | cabal | Who is in a cabal, role, joined at, whether they can vote. | `group_members` + `group_voters` |
| **CabalAccessRequests** | cabal | Both directions: a user asking to join, and a member inviting a user. `direction` = `request` / `invite`, `status` = `pending` / `approved` / `denied` / `revoked` / `expired`. | `group_join_requests` (requests only) |
| **Proposals** | governance | Trade and agent proposals. See [proposals.md](proposals.md). | `proposals` |
| **Vote** | governance | One ballot per voter per proposal. | `votes` |
| **Swaps** | trading | One row per swap attempt, holding the swap state machine `created → submitted → confirmed \| failed` ([trade-execution.md](trade-execution.md)). `proposal_id` or agent intent id, mints, amount, Jupiter `requestId`, signed transaction bytes, `tx_signature`. Default 2026-09-27. | `transactions` |
| **AgentKeys, AgentIntents** | agents | Agent keys, key reveals and intents. Intents emit `agent.intent_created` and run through the same trade engine as proposals ([Flows](backend-platform.md#flows), row 17). | `group_agents`, `group_agent_key_reveals`, `agent_intents` |
| **CabalTxns** | treasury | Double-entry ledger for cabal treasuries: every movement of USDC or a stock token into, out of, or within a treasury. Swaps, fund-to-cabal inflows, redemptions, fees. | `transactions` (swaps only) + parts of `deposits`, `redeem_*` |
| **UserTxns** | treasury | Double-entry ledger for member platform balances: deposits, withdrawals, fund-to-cabal outflows, redemption payouts. | `deposits`, `withdrawals`, `platform_withdrawals`, `redeem_payouts` |
| **CabalPosition** | treasury | Holdings per cabal per asset: units, cost basis. Projection of CabalTxns. Table `cabal_positions`. | `transactions` + `nav_snapshots` |
| **UserPosition** | treasury | Each member's share units in each cabal, amount contributed and withdrawn. Projection of UserTxns and share issuance. | `positions` |
| **CashOutJobs** | treasury | The cash-out job state machine ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). Money movement goes in the ledgers. | `redeem_jobs`, `payout_proofs` |
| **OnrampSessions** | funding | Card-deposit sessions for UX and analytics only; not a ledger. See [deposits-withdrawals.md](deposits-withdrawals.md). | none |
| **ExternalDeposits** | funding | Transfers that reached a treasury outside Monaco's flow, and their bounce status. See [deposits-withdrawals.md](deposits-withdrawals.md#direct-transfers-to-a-cabal-treasury-are-not-allowed). | none |
| **CabalPauses** | funding | One record per trading pause on a cabal, with `reason` (`external_deposit` or `ops`) and when it was resolved. Default 2026-09-27; SQL name and columns in [deposits-withdrawals.md](deposits-withdrawals.md). | `groups` pause columns (never shipped) |
| **Assets** | market | Tradable catalog: symbol, mint, decimals, issuer, kind (xStock, pre-IPO), tradable flag, display name. Filled from every issuer behind the market module's `AssetProvider` strategy ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). | none (fetched live from `api.xstocks.fi`) |
| **PricePoints** | market | One table, `price_points`: one USD price per asset per timestamp, `price_micros` bigint. Written by the one market price poller. Charts, sparklines, valuation and P&L all read it. Pyth dropped. See [price-history.md](price-history.md). Default 2026-09-27. | none |
| **LeaderboardEntries** | ranking | Precomputed board rows per board and range, with `leaderboard_runs` and `cabal_value_snapshots` every 2 minutes (decided 2026-09-27). The cabal P&L curve reads `cabal_value_snapshots`. See [leaderboards.md](leaderboards.md). | `nav_snapshots` |
| **FeedObject** | social | See [feed.md](feed.md). A polymorphic feed item: `kind` (`trade`, `proposal`, `cabal_created`, `member_joined`, `price_move`) plus a reference to the source row. Public feed items carry no money amounts. News is deferred. | none |
| **FeedComment** | social | Comments and replies on any feed object. Replaces proposal comments. See [feed.md](feed.md#comments). | `proposal_comments` (proposal only) |
| **FeedMutes** | social | Per-user mutes on a kind, cabal, stock, user or single item. | none |
| **CabalChat** | social | Chat messages in a cabal, table `cabal_messages`. `parent_id` (one-level threads), `also_in_channel`, denormalized `reply_count` / `last_reply_at`. Soft delete through `deleted_at` (decided 2026-09-27). See [chat.md](chat.md). | `group_messages` |
| **ChatSeen** | social | `chat_seen`: one `last_seen_at` watermark per member per cabal, for "seen by N". Deleted when the member leaves. See [chat.md](chat.md). | `group_members.last_chat_seen_at` (planned, never shipped) |
| **Followers** | social | `follows`: `follower_id`, `followee_id`, `source`, timestamps, `deleted_at`. Unfollow sets `deleted_at`; a re-follow inserts a new row (decided 2026-09-27). Follower and following counts are `count(*)` over live rows. See [followers.md](followers.md). | none |
| **ContactMatches** | social | Monaco users found in a user's phone contacts; seeds follow suggestions. Phone first, X later. Only matches are stored, never unmatched contact hashes. See [auth.md](auth.md#social-graph). | none |
| **Referrals** | referrals | Who referred whom, code, source, status. No reward state: referrals are tracked, not paid (decided 2026-09-27). A referral qualifies at the referred user's first cabal funding of $10 or more. Plus `referral_codes` (random codes only) and aggregated `referral_clicks`. A user's handle works as a code after their first deposit of $10 or more (decided 2026-09-27). See [referrals.md](referrals.md). | none |
| **Notification** | notify | One push to one user: kind, payload, delivered at. Push only, so there is no in-app list and no read state. The row records the intent before the send ([event-bus.md](event-bus.md#consumers-and-handlers)). | none |
| **NotificationBroadcast** | notify | One message fanned out to many users (all members of a cabal, all users, a segment). Individual Notification rows reference it. | none |
| **DeviceTokens** | notify | APNs device tokens per user: `token` (unique), `environment` (`sandbox` / `production`), `last_seen_at`, `disabled_at` when APNs reports it dead. See [notifications.md](notifications.md#tables). | none |
| **Admins** | admin | Admin role per user (`viewer`, `moderator`, `operator`). See [analytics-admin.md](analytics-admin.md#access). | none |
| **AdminActions** | admin | Audit log of every admin action with reason, before and after. Written by the `admin` consumer of `admin.action`. | none |
| **DeadLetters** | admin | `dead_letters`: messages a consumer gave up on (terminated, or past `MaxDeliver`), with the error code and resolve state. Fed from the `DEADLETTER` stream, shown as the admin queue, retried with `monacoctl deadletter retry`. Default 2026-09-27. | none |
| **Events** | platform | Append-only domain event log: `aggregate_type`, `aggregate_id`, `type`, payload with a `v` version field, actor, created at. The source of truth for what happened, and the outbox: `published_at` is set once the relay gets a JetStream ack. Written only through the Unit of Work. See [event-bus.md](event-bus.md#writing-an-event). | none |
| **EventDeliveries** | platform | `(handler, event_id)` primary key, plus the `errs` code of the outcome. One row per event a bus handler has handled; makes redelivery a no-op. Rows older than 30 days are deleted daily. See [event-bus.md](event-bus.md#consumers-and-handlers). | none |
| **IdempotencyKeys** | platform | Stored responses behind the `Idempotency-Key` header on every mutating call ([Thin client](backend-platform.md#thin-client)). | `idempotency_keys` |

`platform` means `internal/platform/db`, `internal/platform/bus` or `internal/platform/httpx` owns the table. No module writes it directly.

Dropped from the 2026-09-26 list:

- **TxnOutbox.** The `events` table is the outbox. See [event-bus.md](event-bus.md).
- **NotificationPreferences.** No mute settings in the MVP (default 2026-09-27; [notifications.md](notifications.md)).
- **Wallets** as one table. Split into UserWallets and TreasuryWallets (below).
- **AssetPrices** (`asset_prices`, `asset_prices_latest`, `price_ticks`). Replaced by `price_points`.
- **Users** columns `follower_count`, `following_count`. Counts are `count(*)` on `follows` (decided 2026-09-27).
- **FollowCounts** and **ReferralUnlocks**. Added and dropped on 2026-09-27. Counts come from `follows`, and `users.first_deposit_at` stays on `users` (decided 2026-09-27).
- **Cabals** columns `trading_paused_at`, `trading_paused_reason`. Moved to the `funding` pause record.
- **CabalMembers** column `last_chat_seen_at`. Moved to `chat_seen`.

`waitlist` is marketing, not product, and no module owns it. Its migrations live with the landing page in `apps/web/migrations`. `schema_migrations` goes with the old runner (`apps/backend/internal/postgres/migrate.go`); atlas keeps its own revision table.

### Types, IDs and migrations

These follow [backend-platform.md](backend-platform.md) and are not repeated here:

- IDs are UUIDv7, branded per aggregate in Go ([Money and types](backend-platform.md#money-and-types)).
- Amounts are integer base units: `money.Micros` for USDC and `money.BaseUnits` for tokens, both unsigned. Ledger entries and P&L use the signed int64 type in `platform/money` (default 2026-09-27). No floats anywhere on the money path.
- Enums (`status`, `kind`, `direction`, `reason`) are named string types with an exhaustive switch in Go.
- Queries are `sqlc`, one directory per module under `queries/`.
- Migrations are atlas versioned SQL under `apps/backend/migrations/`, applied in the deploy's pre-deploy step, forward only ([Decided](backend-platform.md#decided)).

## How it works

### Relationships

```
Users ──< CabalMembers >── Cabals
Users ──  UserWallets              (one per user)
Cabals ── TreasuryWallets          (one per cabal)
Cabals ──< CabalAccessRequests >── Users
Cabals ──< CabalChat
Cabals ──< ChatSeen >── Users
Cabals ──< CabalPauses
Cabals ──< Proposals ──< Vote >── Users
Proposals ──< Swaps                (one live swap per proposal; failed ones allow a retry)
Swaps ──  CabalTxns                (header written on trade.confirmed)
Cabals ──< CabalTxns ──> CabalPosition
Users ──< UserTxns ──> UserPosition  (UserPosition keyed by user + cabal)
FeedObject ──> {Proposal | Swap | Cabal | CabalMember | Asset}
FeedObject ──< FeedComment
Users ──< Followers >── Users
Users ──< Referrals >── Users
NotificationBroadcast ──< Notification >── Users
Users ──< DeviceTokens
Events: written in the same DB transaction as any of the above (Unit of Work), then relayed to NATS
Events ──< EventDeliveries          (one per handler that handled it)
```

An arrow that crosses modules is a foreign key in SQL only. In Go, the module that holds the reference learns about the other row from an event or a query port, never by importing the other module.

### Wallets

Member wallets and treasuries are separate tables with separate owners (default 2026-09-27). `identity` owns `user_wallets` because it creates the wallet at sign-in. `cabal` owns `treasury_wallets` because it creates the treasury with the cabal. On sign-in `identity` looks up the user's existing Privy wallet and records it. It never creates a second one. The relayer and the platform fee wallet are app config in `.env.production`, not rows.

### Double-entry ledgers

Both ledgers follow the same shape: a header row per business transaction, and entry rows that each move an amount of one asset into or out of one account. For every header, entries sum to zero per asset.

```
cabal_txns          id, cabal_id, kind, status, swap_id?, transfer_id?, tx_signature?, created_at
cabal_txn_entries   txn_id, account, asset_id, amount (signed int64, base units)
```

A buy of $50 AAPLx is one header with entries such as `treasury USDC −50`, `treasury AAPLx +0.21`, `venue USDC +50`, `venue AAPLx −0.21`. Positions are sums of entries by account and asset.

Every ledger write happens in the same transaction as the event that announces it (default 2026-09-27). The ledger is a primary write, not a projection. `monacoctl replay --verify` checks it against the events and does not rebuild it. Money event payloads (`trade.confirmed`, `deposit.credited`, `cabal.funded`) still carry every amount the ledger holds, so the check has something to compare.

Who writes which ledger row (default 2026-09-27):

| Money movement | Source of truth for the movement | Ledger row written by |
| --- | --- | --- |
| Swap | `swaps` row in `trading`, driven to `confirmed` or `failed` | `treasury`, as a consumer of `trade.confirmed`: one `cabal_txns` header with entries, in the same transaction as its own event |
| Fund to cabal | `FundCabal` command in `treasury` | `treasury`, in one Unit of Work: a `user_txns` header (member USDC out) and a `cabal_txns` header (treasury USDC in) linked by `transfer_id`, plus share issuance on UserPosition |
| Crypto or card deposit | Deposit poller in `funding` | `treasury`, as a consumer of `deposit.credited`: a `user_txns` header |
| Withdraw to address | `Withdraw` command in `funding` | `treasury`, as a consumer of `withdrawal.confirmed`: a `user_txns` header |
| Cash out | `CashOut` command in `treasury` | `treasury`, directly |

A failed swap has a `swaps` row and no ledger header. The property "both headers on a `transfer_id` always share a status" is a required test ([Testing](backend-platform.md#test-types)).

### Pauses, bans and deletion

- **Trading pause.** `funding` owns the pause record. `trading` and `treasury` read it through `funding`'s query port at check time, so no consumer lag opens a window. An external deposit and an ops pause both write the same record, with a different `reason` (default 2026-09-27; [deposits-withdrawals.md](deposits-withdrawals.md)).
- **Banned user.** `account_status = banned` blocks new actions, but the user can still withdraw and cash out (decided 2026-09-27).
- **Banned cabal.** `cabals.status = banned`. Holdings are sold and USDC returned pro rata through the cash-out path (default 2026-09-27; [analytics-admin.md](analytics-admin.md)).
- **Account deletion.** Cash-out is forced first. Then `users.deleted_at` is set, `account_status` is set to `deleted`, and PII on the row is scrubbed. Ledger rows stay (decided 2026-09-27).

### Deletes

Three tables soft delete (decided 2026-09-27): `users`, `cabal_messages` and `follows`. Each has a `deleted_at timestamptz` column, and reads filter `deleted_at IS NULL`. On `follows`, a unique partial index on `(follower_id, followee_id) WHERE deleted_at IS NULL` lets a re-follow insert a new row. Every other table keeps its current delete behavior. `chat_seen` rows are hard-deleted when a member leaves, since they are not user content. Ledger rows are never deleted.

### Cutover

The old treasuries hold test funds only (decided 2026-09-27). At cutover those funds are wiped: swept to an ops wallet or written off. Members are not cashed out. The new backend starts on an empty database ([Rollout](backend-platform.md#rollout) step 7).

### Asset catalog

`market` refreshes the catalog from each issuer hourly. It flips `tradable` off automatically when an issuer pauses a mint, and an admin can override (default 2026-09-27).

## Naming

- **Cabal everywhere.** Decided 2026-09-27 in [backend-platform.md](backend-platform.md#decided): Go types, tables, event subjects and the new HTTP routes (`/v1/cabals/{id}`) all say `cabal`. `groups` becomes `cabals`, `group_*` becomes `cabal_*`. The legacy `/v1/groups` routes live only as long as the old backend, which ends at [Rollout](backend-platform.md#rollout) step 7.
- **Singular vs plural.** Planned names mix `Vote` with `Proposals`. SQL uses plural throughout (`votes`, `proposals`).

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Swap state on the `cabal_txns` header, entries written in the same update | `trading` owns swaps and `treasury` owns the ledger. One transaction across both breaks the module wall. |
| `funding` writes `user_txns` for deposits and withdrawals | Two writers for one ledger. `treasury` stays the only writer. |
| Unsigned `amount` plus a `direction` column on entries | A signed type makes "entries sum to zero" a plain sum. |
| Ledger rebuilt from events by replay | The ledger is the money authority. Replay checks it instead. |
| One `wallets` table | Two modules create wallets, and a table has one owner. |
| Dead letters on the `DEADLETTER` stream only | The stream keeps 30 days and has no resolve state. |
| Pause columns on `cabals` | `cabals` belongs to `cabal`, and the pause is set by `funding` and ops. |
| Copy old rows into the new schema at cutover | No existing user data carries over (decided 2026-09-27). |

## Open questions

None at the moment.

## Log

- 2026-09-27: Default 2026-09-27: dropped `retired_handles` and `referral_grants`. The unique index on `users.handle` covers deleted rows, so no extra table is needed, and the first deposit of $10 or more is the only unlock.
- 2026-09-27: Decided 2026-09-27: `users.handle` is a unique username every user picks in onboarding, owned by `identity`, with `retired_handles` beside it. `referral_codes` holds random codes only; custom codes are gone and the handle works as a referral code after the first-deposit unlock. `cabal_value_snapshots` are written every 2 minutes, matching the 120 s price poller. Default 2026-09-27 (reversible): `referral_grants` holds the admin unlock.
- 2026-09-27: Decided: soft delete (`deleted_at`) on `users`, `cabal_messages` and `follows`, with a unique partial index for re-follows; `follow_counts` dropped, counts are `count(*)` on live `follows` rows; `referral_unlocks` dropped, `users.first_deposit_at` set by `identity` on the first `deposit.credited`; `chat_seen` stays and is hard-deleted on leave; old treasury funds are test-only and wiped at cutover.
- 2026-09-27: Round-2 decisions applied. Decided by the user: fresh empty database at cutover with no data migration, Privy wallets reused on sign-in; every cabal public; referrals tracked with no reward; banned users can still withdraw and cash out. Defaults (reversible): `swaps` table in `trading` with `treasury` writing `cabal_txns` on `trade.confirmed`; `treasury` writes `user_txns` from `deposit.credited` and `withdrawal.confirmed`; signed int64 type for entries; ledger written with its event and checked by `replay --verify`; pause record owned by `funding`, `cabals.trading_paused_*` removed; `dead_letters` table in `admin` beside the stream; wallets split into `user_wallets` and `treasury_wallets`; one `price_points` table; `account_status` beside `auth_state`; `event_deliveries` keyed by handler, 30-day retention; payload `v` field; hourly catalog refresh with auto-flip; no notification preferences; push only; only matched contacts stored; deletion scrubs PII and keeps ledger rows; banned cabals wind down through cash-out. Stale columns moved to the tables that now own them: `follow_counts`, `referral_unlocks`, `chat_seen`. Dropped the Change column and the "current tables" section, since nothing migrates. No open questions left; status set to Decided.
- 2026-09-27: Reconciled with [backend-platform.md](backend-platform.md). Naming decided (`cabal` everywhere, tables included). Added a Module column with each table's owner. IDs UUIDv7, integer base-unit money, sqlc and atlas migrations referenced from the RFC. `schema_migrations` dropped for atlas. Agent tables kept in the `agents` module; agent intents share the trade engine. Assets filled from every issuer. Events restated as the source of truth; the events-vs-ledgers question narrowed. New open questions: swap row ownership, deposit and withdrawal ledger writes, signed entries, dead-letter storage, wallets ownership, cutover data.
- 2026-09-26: TxnOutbox dropped; `events` doubles as the outbox via `published_at` (NATS decision, [event-bus.md](event-bus.md)). Added `event_deliveries` and `dead_letters`. Resolves the TxnOutbox-scope open question.
- 2026-09-26: Referral codes moved to a `referral_codes` table (custom codes).
- 2026-09-26: Added `device_tokens` and pending `notification_preferences` (notifications proposal); chat adds columns only (`group_messages.parent_id`, `also_in_channel`, reply counters; `group_members.last_chat_seen_at`).
- 2026-09-26: Referrals decided (`referral_code` on users, `referrals`, `referral_clicks`).
- 2026-09-26: Added price tables, leaderboard tables, `cabal_value_snapshots` (leaderboards decision).
- 2026-09-26: Followers decided (`follows` table, counts on users).
- 2026-09-26: Added `admins`, `admin_actions`, cabal `status` (analytics & admin decision).
- 2026-09-26: Added `external_deposits` and cabal trading-pause columns (direct-transfer bounce decision).
- 2026-09-26: Users extended and `contact_matches` added (auth decision).
- 2026-09-26: Added `onramp_sessions` (deposits decision).
- 2026-09-26: Feed decided: FeedComment replaces `proposal_comments`; every proposal and trade gets a FeedObject via outbox. Added `feed_mutes` (per-user mute list) as a planned table.
- 2026-09-26: Planned table list recorded and mapped to current schema. Ledger shape, naming conflict and fold-ins proposed; not yet confirmed.
