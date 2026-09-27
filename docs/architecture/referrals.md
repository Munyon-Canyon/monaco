# Referrals

**Status:** Decided 2026-09-26. No reward for now (decided 2026-09-27). Attribution and tracking only. Reconciled with [backend-platform.md](backend-platform.md) 2026-09-27: owned by the `referrals` module, flow 25 in [`flows.tsv`](backend-platform.md#flows), built in [Rollout](backend-platform.md#rollout) step 6.

## Decision

Every user has a referral link, `https://monacolabs.xyz/r/<code>`, with a random code from signup. After their first deposit of $10 or more, their handle also works as a code (`/r/kaicenat`). If the recipient has the app, the link opens it straight into the app (Universal Link). If they do not, the link's landing page **copies the link to the clipboard** when they tap "Get Monaco", then sends them to the App Store. On first launch the app offers a one-tap **Paste invite** button, reads the link, and attaches the referral to the new account. A manual "Have a referral code?" field in onboarding catches anything the clipboard misses.

Referrals carry **no reward** for now (decided 2026-09-27). The system attributes each new user to a referrer and tracks whether the referral qualifies, so the funnel is measurable and a reward can be added later without new data.

No third-party attribution service, no fingerprinting.

The `referrals` module owns `referral_codes`, `referrals` and `referral_clicks` ([Repository layout](backend-platform.md#repository-layout)). It reads user facts (created at, `auth_state`, status, `handle`, `first_deposit_at`) through the `identity` module's read-only query port, and it changes other modules only through events ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)).

## Why

- iOS cannot carry a link through an App Store install. Something has to bridge the gap between the tap and the first launch.
- The clipboard is **deterministic**: the code the user tapped is exactly the code the app reads. Probabilistic matching (IP, device model, timing) is wrong on shared Wi-Fi and VPNs, and Apple's privacy rules restrict fingerprinting.
- No vendor cost or SDK. Branch and similar stay an option if public links become a big channel.

## Codes

Every user has a **random code** from account creation. After the unlock, their **handle** also works as a code. Both resolve to the same user; the link format is the same: `monacolabs.xyz/r/<code>`.

**The handle comes from onboarding, not from referrals** (decided 2026-09-27). Every user picks a unique handle as the first onboarding step. `identity` owns it on `users.handle`, with its naming, impersonation, change-limit and retirement rules ([auth.md](auth.md#handle)). `referrals` never creates, validates or gates a handle. It reads handles through `identity`'s query port.

**Random code.** 8 characters from an unambiguous alphabet (no `0/O`, `1/I/l`), e.g. `K7M4QX2P`. Created by the `referrals` consumer of `user.created` (flow 1), so it exists moments after the account does. Never changes, never reused. Handle rules forbid anything shaped like a random code, so the two namespaces never overlap.

**Handle as a code**, e.g. `monacolabs.xyz/r/kaicenat`:

- **Unlocked by a first deposit.** A handle works as a referral code only after the user's first deposit is credited: USDC recorded in their user ledger, crypto or card ([deposits-withdrawals.md](deposits-withdrawals.md)), of at least $10 (`10_000_000` micros as a `money.Micros`, kept as the default 2026-09-27), so a one-cent deposit does not count. The unlock is permanent: withdrawing the money later does not take it away. It is stored as `users.first_deposit_at` (decided 2026-09-27). `identity` owns that column and sets it in its consumer of `deposit.credited` (flow 5), never from the client. The write is a guarded update, `WHERE first_deposit_at IS NULL`, so only the first deposit of at least $10 sets it. Smaller deposits are skipped. `referrals` reads it through the `identity` query port.
- **Resolving.** `/r/<code>` first looks up `referral_codes`. On a miss, `referrals` asks the `identity` query port for the user whose handle matches, case-insensitive. The handle resolves only when that user has `first_deposit_at`. The first deposit is the only unlock. A locked handle is treated as an unknown code.
- **Changed handles.** A rename frees the old handle at once ([auth.md](auth.md#handle)), so links that used it stop resolving to this user. The random-code link never changes. A handle an admin revoked stops resolving; the page shows the generic invite. Referrals already attributed through a handle stay with the original referrer.

**API.**

| Route | Does |
| --- | --- |
| `GET /v1/me/referral-code` | The user's random code and link, plus the handle link once unlocked. |

Handle availability and changes are `identity` routes ([auth.md](auth.md#handle)).

The app shows the handle link when it is unlocked and the random link otherwise. Both always work once unlocked. Before the first deposit, the referral screen shows "Make your first deposit to use @handle as your invite link" with a Deposit button, which doubles as a deposit nudge.

## Flow

### Recipient already has the app

1. Tap link in Messages, WhatsApp, etc. iOS matches the `/r/*` path in `monacolabs.xyz/.well-known/apple-app-site-association` and opens the app with the URL.
2. Signed in already: show the referrer's profile with a Follow button. No referral attached (they are not a new user).
3. Not signed in yet: store the code locally, continue into login and onboarding, attach after account creation (below).

Universal Links do not fire from every surface: in-app browsers (X, Instagram, TikTok) and a URL typed into Safari open the web page instead. The landing page therefore always has an **"Open in Monaco"** button as well, which works when the app is installed.

### Recipient does not have the app

```
tap link ─► monacolabs.xyz/r/K7M4QX2P
            "Alex invited you to Monaco"  [Get Monaco]
                    │ tap (user gesture)
                    ├─ navigator.clipboard.writeText("https://monacolabs.xyz/r/K7M4QX2P")
                    ├─ POST /v1/referrals/clicks { code }   (count only, Idempotency-Key per tap)
                    └─ redirect ─► App Store ─► install ─► first launch
                                                           │
            app: UIPasteboard.general.detectPatterns([.probableWebURL]) → true?
                                                           │ yes
            "Were you invited?"  [Paste invite]  (SwiftUI PasteButton)   [Skip]
                                                           │ tap
            parse URL → host monacolabs.xyz, path /r/<code> → keep code locally
                                                           │
            Apple / Google login → POST /v1/auth/session (new user)
                                                           │
            POST /v1/me/referral { code, source: "clipboard" }
```

Details that matter:

- **The copy must happen on a tap.** Browsers only allow clipboard writes inside a user gesture, so the page uses a button rather than copying on load.
- **No iOS paste prompt.** Reading the clipboard programmatically shows "Monaco would like to paste from Safari". Two things avoid it: `detectPatterns` checks whether the clipboard holds a URL *without* reading it, and SwiftUI's `PasteButton` (`UIPasteControl`) reads only when the user taps it, with no alert. The app never reads the clipboard silently.
- **Ask once.** The paste screen shows only on the first launch, before signup, and only if a URL is detected. Skip dismisses it for good. The referral field in onboarding remains.
- **Validate what was pasted.** Accept only `https://monacolabs.xyz/r/<code>` with a well-formed code. Anything else is ignored; nothing from the clipboard is sent anywhere else or shown raw.
- **Keep the code across login.** Stored in `UserDefaults` until attached or the user finishes onboarding, then cleared.

### Manual fallback

Onboarding has a "Have a referral code?" field (also reachable from the paste screen's Skip). Same attach call with `source: "manual"`. This catches clipboard overwrites (user copied something else before opening the app), a different device, or a friend reading the code aloud.

## Attaching a referral

`POST /v1/me/referral { code, source }`, `source` = `universal_link` | `clipboard` | `manual`. This is the `AttachReferral` command, with an `Idempotency-Key` like every mutating call ([Thin client](backend-platform.md#thin-client)).

Backend checks, in one `uow.Do` ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). The user facts come from the `identity` query port:

- Code resolves to a user ([Codes](#codes)): a random code in `referral_codes`, or an unlocked handle, through the `identity` query port. The owner is not the caller.
- Caller has no referral yet (`referrals.referee_id` unique).
- Caller is new: account created less than 7 days ago (the attribution window, kept as the default 2026-09-27) and `auth_state` is not yet `ONBOARDING_COMPLETED` for more than 24 hours. Stops old users claiming a friend's code later.
- Referrer is not `BANNED` or `DELETED`.

`AttachReferral` is the only attribution path (default 2026-09-27). Universal Link, clipboard and manual codes all arrive after the session exists, so one command covers all three sources. The `referrals` consumer of `user.created` (flow 1) only mints the random code; it never attributes.

On success: insert `referrals` row (`status = attributed`) and append the `events` row `referral.attributed` in the same transaction. Consumers do the rest. `social` auto-follows both ways (`follows.source = 'referral'`, [followers.md](followers.md)); `referrals` never writes `follows`. `notify` tells the referrer ("Sam joined from your invite"). Failure returns an `errs` code whose table message the app shows in a toast ("That code isn't valid").

`referral.attributed` has its own row in the flows table, with `social` as a consumer (platform default 2026-09-27).

## Qualification

A referral moves `attributed → qualified`, or `rejected`. Nothing is paid or granted on qualifying.

- **Qualified** when the referee first funds a cabal with at least $10 (`10_000_000` micros), default 2026-09-27. Funding a cabal is real use, which a deposit alone is not. Triggered by the `referrals` consumer of `cabal.funded` (flow 7), never from the client. Qualifying appends `referral.qualified`, whose consumers are `notify` and `analytics` (flow 25). `deposit.credited` (flow 5) only unlocks the handle as a code, through `identity`'s consumer. `referrals` does not consume it.
- **Honest counts.** Qualification also requires the referee's phone to be verified ([auth.md](auth.md)); verified phones are unique, so one person cannot inflate a referrer's numbers with extra Apple IDs. Bursts from one referrer are flagged for review in the admin panel.
- **If a reward comes later**, it is a new consumer of `referral.qualified`. A cash reward moves money, so that consumer would live in the module that owns member balances, not in `referrals`.

## Data

```
referral_codes           -- random codes only; handles live on users.handle (identity)
  code          text PRIMARY KEY          -- stored lowercase; lookups lowercase the input
  user_id       → users UNIQUE
  created_at

referrals
  id, referrer_id → users, referee_id → users UNIQUE, code (random code or handle, as used), source,
  status (attributed | qualified | rejected), reject_reason,
  created_at, qualified_at

referral_clicks          -- landing-page taps, for the funnel only
  code, day, clicks      -- aggregated per code per day; no IP, no user agent stored
```

The handle unlock is `users.first_deposit_at`, owned by `identity` ([Codes](#codes)).

## Web

- `monacolabs.xyz/r/<code>` is a server-rendered route in `apps/web` (Cloudflare function): looks up the referrer's display name and photo from the API, renders the page, and sets Open Graph tags so the link preview in Messages shows "Alex invited you to Monaco" with their photo.
- Unknown code: generic "Get Monaco" page, still links to the App Store, copies nothing.
- `apple-app-site-association` lists `/r/*` for `com.monaco.app`. The app declares `applinks:monacolabs.xyz` in its entitlements.

## Analytics

PostHog funnel ([analytics-admin.md](analytics-admin.md#product-analytics-posthog)): `referral_page_viewed` → `referral_get_tapped` → (app) `invite_paste_shown` → `invite_pasted` / `invite_skipped` → `login_completed` → (server) `referral_attributed` → `referral_qualified`. Plus attribution by `source`, so the clipboard hit rate is measurable and the decision to add a vendor later is based on numbers.

## Alternatives considered

| Alternative | Why not (for now) |
| --- | --- |
| Invite by phone number, match on phone verification | Exact, but only works for invites sent to a specific contact, not a posted link. Can be added later on top of this. |
| Branch / AppsFlyer / Adjust | Higher match rate on posted links, but cost, an SDK and a third party in the attribution path. Revisit if clipboard hit rate is low. |
| Own IP + device matching | Wrong on shared networks, and Apple restricts fingerprinting. |
| Silent clipboard read on launch | Triggers the iOS paste alert unprompted, which looks like spyware. `PasteButton` avoids it. |

## Open questions

None at the moment. When an Android app exists, the Play Install Referrer API replaces the clipboard step.

## Log

- 2026-09-27: Default 2026-09-27: dropped `referral_grants` and the admin grant. The first deposit of $10 or more is the only unlock. Old handles no longer resolve after a rename.
- 2026-09-27: Decided 2026-09-27: every user picks a unique handle in onboarding, owned by `identity` on `users.handle`. The random code is still minted at signup. After the first deposit of $10 or more, the handle also works as a referral code. Custom codes, `ClaimReferralCode` and the availability route are gone; handle rules, changes and admin revoke or reassign move to [auth.md](auth.md#handle). `referral_codes` holds random codes only. Default 2026-09-27 (reversible): the admin deposit-skip becomes a `referral_grants` row.
- 2026-09-27: Decided: dropped `referral_unlocks`. `users.first_deposit_at` stays on `users`, set by `identity`'s consumer of `deposit.credited` with a guarded update, and read by `referrals` through the `identity` query port. The admin grant is now an admin-assigned custom code that skips the deposit check.
- 2026-09-27: Decided no reward for now; attribution and tracking only. The `rewarded` status and `rewarded_at` are dropped. Defaults applied: `AttachReferral` is the only attribution path and the `user.created` consumer only mints the random code; a referral qualifies at the referee's first cabal funding of $10 or more; the custom code is the profile handle; the $10 unlock minimum and 7-day window stay. `referral.attributed` has a flows row. Closed the reward, qualification, handle, minimum, window and attribution-path questions.
- 2026-09-27: Reconciled with [backend-platform.md](backend-platform.md). Owner is the `referrals` module, flow 25, rollout step 6. `users.first_deposit_at` moves to a `referrals`-owned `referral_unlocks` table. Random codes are minted by the `user.created` consumer. Auto-follows move to a `social` consumer of `referral.attributed`. Claim and attach are commands with `Idempotency-Key`; refusal reasons are `errs` codes. Minimum deposit is integer micros. Added the attribution-path open question.
- 2026-09-26: Outbox rows replaced by `events` rows delivered over the NATS event bus ([event-bus.md](event-bus.md)).
- 2026-09-26: Custom codes gated behind a first credited deposit (minimum amount, permanent unlock, admin override).
- 2026-09-26: Custom referral codes (`/r/kaicenat`). Codes move from a `users` column to a `referral_codes` table; retired custom codes keep resolving; X-handle impersonation guard; admin revoke/reassign.
- 2026-09-26: Initial decision. Referral links with clipboard handoff for deferred deep linking (option 2), manual code fallback, no attribution vendor.
