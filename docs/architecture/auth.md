# Auth & onboarding

**Status:** Decided 2026-09-26; login and account-standing rules decided 2026-09-27; login methods changed 2026-09-29; Apple and Google added 2026-10-02 and removed from the login screen 2026-10-08 (#4140). X follow import is deferred past MVP. The backend shape (modules, events, errors, rollout) follows [backend-platform.md](backend-platform.md), which wins where the two differ.

## Decision

- **Login is SMS OTP or email OTP**, through Privy, in every build including production (decided 2026-09-29). Apple and Google sign-in were added under the OTP form on 2026-10-02 (#541) and removed from the login screen on 2026-10-08 (#4140), so login is SMS OTP or email OTP only. Agents and simulators sign in with OTP, the same way as users.
- **Two Privy apps** (decided 2026-10-02): local and staging builds use the dev Privy app, so the staging backend verifies dev tokens; production builds use the production Privy app. A Release build whose environment has no Privy app refuses to launch.
- After first login, onboarding asks for three things on separate screens:
  1. **Username (handle)**, unique across Monaco and required (decided 2026-09-27). It lives on `users.handle` and `identity` owns it. See [Handle](#handle).
  2. **Phone number**, verified by SMS code and linked to the same Privy user.
  3. **Twitter / X account**, linked to the same Privy user through OAuth.
- Phone and X are used to build the user's **social graph**: match their phone contacts and X follows against Monaco users, recommend people to follow, and seed an internal graph.
- The `users` row carries an **`auth_state`** column for onboarding progress (`CREATED`, `AWAITING_PHONE`, `AWAITING_SOCIALS`, `ONBOARDING_COMPLETED`) and a separate **`account_status`** column for standing (`active`, `suspended`, `banned`, `deleted`) (default 2026-09-27). The app reads both to decide what to show. Notifications and in-app banners read `auth_state` to nudge users to finish giving phone and X. Analytics read its history to see how users engage socially.
- **Suspended and banned users can still withdraw and cash out** (decided 2026-09-27). The money is theirs.
- **No migration for existing users** (decided 2026-09-27). All user data is wiped at cutover and the new backend starts on an empty database. A returning user signs in with an SMS or email code and gets a fresh `users` row. Their Privy wallet lives on-chain and in Privy, so sign-in reuses it and never recreates it.

## Why

- **OTP login** needs no Apple or Google account and works the same in every environment. Apple ships with Google because App Store guideline 4.8 requires it once any third-party social login is offered.
- **Phone and X are for the graph.** A phone used to sign in is not stored as the contact-matching number until the user finishes the phone step. A user who signed up by SMS has already proven that number to Privy, so the phone step only confirms it and never asks for it again. The X link stays a separate, skippable ask, and the app nudges it later.
- **Stored state columns** give the app, notifications and analytics a single, indexable answer to "where is this user", instead of each re-deriving it from linked accounts. Two columns, not one, because onboarding progress and account standing change independently: a banned user keeps their onboarding state, and "banned users who never gave a phone" stays answerable.

## Login

Privy Swift SDK:

```swift
try await privy.sms.sendCode(to: phone)
try await privy.sms.loginWithCode(code, sentTo: phone)
try await privy.email.sendCode(to: email)
try await privy.email.loginWithCode(code, sentTo: email)
try await privy.oAuth.login(with: .apple, appUrlScheme: "monaco")
try await privy.oAuth.login(with: .google, appUrlScheme: "monaco")
```

Closing the Apple or Google sheet returns to the login screen with no toast. Any other sheet failure is toasted.

Then the app calls `POST /v1/auth/session` with the Privy access token. The route is public, because no account exists on the first call, and takes no `Idempotency-Key`: finding or creating the user by `privy_user_id` is already idempotent. It is limited to 60 calls a minute per IP. The backend:

1. Verifies the token and reads the Privy user. A Privy outage is `PrivyUnavailable`.
2. Picks `login_provider` from what Privy links, in this order: `sms` for a phone, `email` for an email, `apple`, `google`. A Privy user with none of them is `LoginMethodNotAllowed`.
3. Refuses a deleted account with `AccountDeleted` and creates no new row for it.
4. Reuses the user's one member wallet, and asks Privy for one only when no `user_wallets` row exists (never recreates). That call happens before the transaction.
5. In one transaction, creates the `users` row if the `privy_user_id` is new, attaches the wallet, refreshes `email`, and resyncs the links (see [`auth_state`](#auth_state)). A stored wallet whose address differs from Privy's is `WalletMismatch` and is never overwritten.
6. After the commit, publishes the `me_changed` hint when anything changed, and returns `Me`. A suspended or banned user gets `Me` too, so the app can show the notice screen.

In the rewrite this is [flow 1](backend-platform.md#flows), owned by the `identity` module. A first sign-in appends `user.created`; every state change appends `user.auth_state_changed`. Both go through `uow.Do` in the same transaction as the row ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). The `analytics`, `referrals` and `social` consumers react.

Privy dashboard: enable SMS, email, Apple and Google as **login** methods on both Privy apps. Bundle `com.monaco.app` stays on both Privy iOS clients, and each iOS client allows the `monaco` app URL scheme, which the Apple and Google sheets redirect to.

## Onboarding

```
SMS or email login, then handle picked (required)
        │
        ▼
   ┌─────────┐   phone verified    ┌────────────────────┐   X linked    ┌───────────────────────┐
   │ CREATED │ ──────────────────► │ AWAITING_SOCIALS   │ ────────────► │ ONBOARDING_COMPLETED  │
   └─────────┘                     └────────────────────┘               └───────────────────────┘
        │ skip phone                        ▲                                     ▲
        ▼                                   │ phone verified later                │
   ┌────────────────┐ ──────────────────────┘                                     │
   │ AWAITING_PHONE │ ────────── X linked, phone still missing: stays AWAITING_PHONE
   └────────────────┘
```

**Screen 1: handle.** User types a handle. The app checks it live with `GET /v1/handles/{handle}/availability`, then calls `PUT /v1/me/handle` (`SetHandle`). This screen has no Skip. Until `users.handle` is set, the app shows only this screen, and the phone and socials commands refuse with `HandleRequired`. Rules are in [Handle](#handle).

**Screen 2: phone.** `GET /v1/me` carries `login_provider`. When it is `sms`, the app skips the number entry and the Privy code: it calls `POST /v1/me/onboarding/phone` straight away, and the backend stores the sign-in number it reads from Privy. If another user already holds that number, the call fails with `phone_not_linked` and the app shows the form below. For `email`, `apple` and `google`, the user enters a number. App calls `privy.sms.sendCode(to:)`, then `privy.sms.linkWithCode(_:)`. Linking through Privy means the number is **verified** before Monaco trusts it; this matters because contact matching on an unverified number would let anyone claim someone else's number. App then calls `POST /v1/me/onboarding/phone`; the backend reads the verified phone from the Privy user (server-side), never from the request body, stores it, and advances state.

**Screen 3: X.** App calls `privy.oAuth.link(with: .twitter, appUrlScheme:)`. Then `POST /v1/me/onboarding/socials`; backend reads the linked X account (username and numeric X user id) from Privy server-side and stores it.

**Fake X for dev users.** An agent or an XCUITest cannot sign in on x.com, so outside production a dev user (Privy email `dev-<suffix>@example.com`, made by `monacoctl dev token --user new`) can link a fake X account through `POST /v1/dev/me/x-link`, with an optional `username` that defaults to `dev_x_<suffix>`. The backend stores it in `dev_x_links`, and a decorator on the Privy adapter reports it as the user's X account, with the X user id `dev:<suffix>`. `POST /v1/me/onboarding/socials` then links it through the same code as a real account. `DELETE /v1/dev/me/x-link` removes it, so the next session unlinks X. Production never mounts either route: the router answers every `x-dev-only` operation with `not_found` before auth, and the module wires no handler and no decorator.

**Skip.** The phone and X screens have Skip. The handle screen does not. Skipping phone sets `AWAITING_PHONE`; skipping X (with phone done) sets `AWAITING_SOCIALS`. The user lands in the app either way.

**Already linked elsewhere.** Privy allows one account per type per user. If the phone or X account is linked to a different Privy user, linking fails; the screen says so and offers Skip.

All three onboarding routes are `identity` commands. Like every mutating call, they take an `Idempotency-Key` header ([Thin client](backend-platform.md#thin-client)). A refusal returns a stable `code` from the `errs` table as `application/problem+json` ([Errors](backend-platform.md#errors)), and the app shows its `message` in a toast.

### Handle

Every user has exactly one active handle (decided 2026-09-27). It shows as `@handle` on the profile, and after the first-deposit unlock it also works as the user's referral code in `monacolabs.xyz/r/<handle>` ([referrals.md](referrals.md#codes)). The display name stays a separate, non-unique label ([product.md](../product.md)).

- 3 to 20 characters: letters, digits, underscore. Stored lowercase in `users.handle`; matching is case-insensitive, so `KaiCenat` and `kaicenat` are the same handle.
- Unique across every `users` row, deleted ones included. The unique index on `users.handle` has no `deleted_at` filter, so a deleted user's handle is never reused. That one index settles every race: a claim and a rename each write the same column, so two writers of one handle collide on it and one gets `taken`.
- **Blocked:** a reserved list (`admin`, `monaco`, `monacolabs`, `support`, `help`, `api`, `app`, `r`, `official`, `team`, …), a profanity list, and anything that looks like a random referral code (exactly 8 characters from the random-code alphabet). That last rule keeps handles and random codes in disjoint namespaces, so `/r/<code>` never matches both.
- **Impersonation.** A handle equal to **another user's linked X handle** is refused. Your own X handle is always allowed once X is linked, even if it matches a reserved or popular string.
- **Change limit:** once per 30 days. Changing frees the old handle immediately. Links that used it stop resolving to this user, and anyone can claim it.
- **Admin.** Moderators can revoke a handle (the user must pick a new one on next launch) or reassign it to a verified owner, with a reason, audited through `admin.action` ([analytics-admin.md](analytics-admin.md#actions)).

`GET /v1/handles/{handle}/availability` returns `available`, or why not (`taken`, `reserved`, `invalid`, `too_soon`). It is rate-limited. Each reason is an `errs` code, so the availability body and the `SetHandle` refusal share one closed list ([Errors](backend-platform.md#errors)). `referrals` and `social` read handles through `identity`'s query port.

## `auth_state`

| State | Meaning | App shows |
| --- | --- | --- |
| `CREATED` | Logged in, onboarding not yet finished or not yet reached. `handle` may still be null. | Onboarding flow, from the handle screen if `handle` is null, else from the phone screen. |
| `AWAITING_PHONE` | Finished onboarding but skipped phone. | App, plus "Add your number to find friends" banner. |
| `AWAITING_SOCIALS` | Phone verified, skipped X. | App, plus "Connect X to find people you follow" banner. |
| `ONBOARDING_COMPLETED` | Phone and X both linked. | App, no nudges. |

## `account_status`

| Status | Meaning | App shows | Allowed |
| --- | --- | --- | --- |
| `active` | Normal account. | App. | Everything. |
| `suspended` | Temporarily blocked (e.g. under review). | Read-only app with a notice. | Reads, withdraw, cash out. |
| `banned` | Permanently blocked. | A notice with only the withdraw and cash-out screens. | Withdraw, cash out. |
| `deleted` | User requested deletion. PII scrubbed; row kept for ledger integrity. | Nothing; login creates no new row for the same Privy id without ops action. | Nothing. |

Rules:

- **Only the backend writes `auth_state` and `account_status`**, from verified Privy data or ops actions. The client never sends either.
- Each column is a state machine in `identity/domain`: a Go type with a `transitions` table and a pure `Next(from, event)` ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). The adapter applies it as a guarded update (`WHERE auth_state = $expected`, or `account_status`) inside `uow.Do`. An `auth_state` change appends `user.auth_state_changed` with from, to, and cause (`onboarding`, `link`, `unlink`). That history is what analytics reads. An `account_status` change is an admin command and appends its own event plus `admin.action` ([analytics-admin.md](analytics-admin.md#actions)).
- `suspended` and `banned` are enforced in the auth middleware on every request, not only in the app. They block funding, voting, proposing and commenting with a `KindForbidden` code (403). Withdraw (flow 15) and cash out (flow 14) stay open to them (decided 2026-09-27).
- If a user unlinks phone or X in Privy, the next session call clears its columns and moves them back to the matching `AWAITING_*` state, with cause `unlink`. A phone or X linked in Privy after onboarding is stored at the next session call with cause `link`. During `CREATED` the onboarding commands own that step. The app does not store the sign-in phone when it opens the session, because that would skip the X step. A number or X account another user already stores is skipped, and the next call tries again.

### Deletion

"Delete my account" requires a zero position first (default 2026-09-27). The app sends the user through cash out for every cabal and withdraw for the platform balance. Deletion is refused with a `KindBlocked` code until nothing is left. Then `identity` sets `deleted_at` and `account_status = deleted`, and scrubs PII (`email`, phone columns, X columns, display name, photo). The `users` row keeps its handle, so no one else can claim it. This is a soft delete (decided 2026-09-27): reads filter `deleted_at IS NULL`. Ledger rows and the `users` row stay, so every `cabal_txns` and `user_txns` entry still points at a real id. Ledger rows are never deleted.

### Nudges

A daily poller in the `identity` module selects users in `AWAITING_PHONE` / `AWAITING_SOCIALS` and appends `user.nudge_due` events that the `notify` consumer turns into pushes ("3 of your contacts are on Monaco"). Like every poller it holds a Postgres advisory lock per tick, so it runs on one worker ([Deploy rule 1](backend-platform.md#deploy-and-observability)). Banners are rendered by the app from `auth_state` directly. Copy and frequency caps belong to [notifications.md](notifications.md). There is no incentive for completing: referrals carry no reward for now (decided 2026-09-27, [referrals.md](referrals.md)).

## Social graph

The `social` module owns contact matching, `contact_matches` and the `/v1/me/contacts/*` routes ([Repository layout](backend-platform.md#repository-layout)). It reads verified phone hashes and X user ids through the read-only query port `identity` exports in `module.go`, never through `identity`'s packages ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)).

### Phone contacts

1. With the user's permission (iOS Contacts prompt, with Monaco's own explainer screen first), the app reads phone numbers from the address book.
2. The app normalises each to E.164 and hashes it with SHA-256 before upload. Raw numbers and names never leave the device.
3. `POST /v1/me/contacts/match` with the hash list. The backend compares against the hashes of **verified** phone numbers of Monaco users (`users.phone_hash`) and stores matches in `contact_matches (user_id, matched_user_id, source = 'phone')`.
4. The app shows "Friends on Monaco" with Follow buttons.
5. Unmatched hashes are not stored (default 2026-09-27). When a new user verifies their phone later, existing users who had them in contacts are not told. Less growth, less stored PII.

Phone number hashes are brute-forceable (the number space is small), so hashing is a courtesy, not a privacy guarantee. Treat `phone_hash` as PII. The slog redaction list already covers `phone`; `phone_hash` and `x_user_id` join it in the PR that adds them ([Logs as evidence rule 7](backend-platform.md#rules)).

### X follows

Deferred past MVP (default 2026-09-27). Phone-contact matching ships first; X follows follow once the paid X API tier is worth it. X linking stays in onboarding, so `x_user_id` is ready when matching ships. The design for then:

The Privy Swift SDK **does not return the user's X OAuth tokens** to the app ("not yet supported in the Swift SDK"), so the app cannot call the X API as the user. Instead:

1. Backend reads the linked X user id from Privy.
2. Backend calls the X API through a `social` adapter (anti-corruption layer, circuit breaker and retry, per [Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)) with Monaco's own app credentials, `GET /2/users/{id}/following`, and matches returned ids against `users.x_user_id`.
3. Matches go into `contact_matches` with `source = 'x'`.
4. Re-run weekly and on user request, rate-limited.

The following endpoint is on a paid X API tier with low monthly caps, which is why it waits.

### Internal graph

- `contact_matches` is the "people you may know" seed. Accepting a recommendation creates a row in `follows` ([followers.md](followers.md)) with `source` set to `phone` or `x`.
- Further recommendations come from inside Monaco: members of the same cabals, people who comment on the same feed items, followers of followers.
- Engagement analytics read `events` for follows, contact-match accepts, invites sent, and `auth_state` transitions, joined to the source of each follow (`phone`, `x`, `cabal`, `feed`, `suggested`).

## Data

`users` belongs to `identity`; `contact_matches` belongs to `social`. `users` gains:

| Column | Notes |
| --- | --- |
| `auth_state` | Enum above, default `CREATED`, indexed. |
| `account_status` | Enum above, default `active`, indexed. |
| `auth_state_changed_at` | For "stuck in onboarding for 7 days" queries. |
| `login_provider` | `sms`, `email`, `apple` or `google`. Sign-in uses `sms` or `email` until #541. |
| `email` | The email Privy links, from email login, or from Apple or Google once #541 lands. Refreshed at every sign-in. Apple may give a private relay address. |
| `phone_e164`, `phone_hash`, `phone_verified_at` | Verified via Privy only. `phone_hash` unique. |
| `x_user_id`, `x_username`, `x_linked_at` | From Privy's linked X account. `x_user_id` unique. |
| `handle` | Unique username picked in onboarding ([Handle](#handle)). Lowercase, unique index `WHERE handle IS NOT NULL` with no `deleted_at` filter, so deleted users keep their handle. Null only before onboarding picks it or after an admin revoke. Decided 2026-09-27. |
| `first_deposit_at` | Set by `identity`'s consumer of `deposit.credited` (flow 5) on the first deposit of at least $10, with a guarded update `WHERE first_deposit_at IS NULL`. `referrals` reads it through the `identity` query port to unlock the handle as a referral code ([referrals.md](referrals.md#codes)). Decided 2026-09-27. |
| `deleted_at` | Soft delete (decided 2026-09-27). Set with `account_status = deleted`. Reads filter `deleted_at IS NULL`. |

New tables: `contact_matches (user_id, matched_user_id, source, created_at, dismissed_at)`, owned by `social`. Followers is its own decision.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Keep email / SMS OTP login | More steps than one-tap Apple/Google, and SMS login costs money per send. |
| Phone number collected in a text field without verification | Lets a user claim someone else's number and harvest their contacts' graph. |
| Upload raw contacts | Privacy and App Store risk. Hash on device; upload numbers only. |
| X follow graph via the user's OAuth token | Not available in the Privy Swift SDK. Would require running a separate X OAuth flow outside Privy. Kept as a fallback if the app-credential route is too expensive. |
| Derive onboarding state from linked accounts on every read | Every screen, notification query and analytics query repeats the logic. One stored column, written in one place, is simpler. |

## Gap between this and the code

Today: email OTP and SMS OTP login through Privy, with Apple and Google beside them; `users` has `id`, `privy_user_id`, `display_name`, `profile_photo_url`, `created_at`. No state column, no phone, no X, no contacts.

There is no migration path for existing users (decided 2026-09-27). The new backend starts on an empty database, so there is no backfill of `auth_state` and no dual login period.

The rewrite builds this in [Rollout](backend-platform.md#rollout) step 3 (`identity`, flow 1) and step 6 (`social` contact matching, `notify` nudges).

## Open questions

None.

Log: [log/auth.md](log/auth.md).
