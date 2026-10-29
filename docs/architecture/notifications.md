# Notifications

**Status:** Decided 2026-09-27. The delivery path is APNs direct from the backend, as a consumer on the event bus. Which events notify, caps, badge and list were set as defaults on 2026-09-27, and the pause, follower, chat and nudge pushes were decided 2026-09-27; push copy is a tracked TODO. The backend shape (modules, bus, errors, rollout) follows [backend-platform.md](backend-platform.md), which wins where the two differ.

## Decision

The backend sends push notifications straight to Apple Push Notification service (APNs) over HTTP/2, using token-based auth (one `.p8` signing key). There is no third-party push service. The iOS app only asks the user for permission, gets a device token from APNs, and registers that token with the backend. The app never sends a push itself.

Pushes are always side effects of events on the NATS bus ([event-bus.md](event-bus.md)). Whatever changes state, such as a trade confirm or a proposal status change, appends an `events` row in the same `uow.Do` transaction, and the relay publishes it. The `notify` module owns notifications, device tokens and APNs ([Repository layout](backend-platform.md#repository-layout)). Its one durable pull consumer on `EVENTS`, named `notify`, subscribes to the event types that notify, turns each into **Notification** rows (see [data-model.md](data-model.md)), and APNs sends from those rows. No code path calls APNs inline, and emitters do not know whether their event notifies anyone.

## Why

- **The app is iOS only.** Firebase Cloud Messaging and OneSignal pay off when one API has to cover Android and web too. For iOS alone they add a vendor, an SDK in the app and another set of credentials, all to wrap the same APNs call.
- **Events over direct calls.** Same reasoning as trade execution. A push for a trade that rolled back, or a trade that confirmed but never notified because APNs was down, are both worse than a short delay. Events written in the state change's transaction and delivered by JetStream give at-least-once delivery with no phantom pushes.
- **Notification rows are the source of truth.** "Delivered at", dedupe and caps all come from the table. APNs is one delivery channel off that table. An in-app list or badge can read the same rows later without a new write path.

## How it works

### One-time Apple setup

1. Apple Developer → Certificates, Identifiers & Profiles → **Keys** → create a key with **Apple Push Notifications service (APNs)** enabled. Download the `.p8` file (Apple lets you download it only once). Note the **Key ID** and your **Team ID**.
2. Xcode → target `Monaco` → Signing & Capabilities → add **Push Notifications**. This adds the `aps-environment` entitlement. Also add **Background Modes → Remote notifications** if silent pushes are needed later.
3. Add to `.env.local` through dotenvx (`just encrypt`), and to `.env.production` for prod:
   - `APNS_KEY_P8`: the `.p8` file contents
   - `APNS_KEY_ID`
   - `APNS_TEAM_ID`
   - `APNS_TOPIC=xyz.monacolabs.app` (the bundle id)

One key works for both sandbox and production, and for every app on the team.

### Device registration (iOS)

The app asks for permission at a moment the user understands, after the user's first create or join of a cabal, through a sheet shown once per install (#2143), never at cold launch. Once the user allows it, the app registers for remote notifications and posts the token to the backend.

```swift
// AppDelegate, wired into the SwiftUI App with @UIApplicationDelegateAdaptor
func requestPushPermission() {
    UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { granted, _ in
        guard granted else { return }
        DispatchQueue.main.async { UIApplication.shared.registerForRemoteNotifications() }
    }
}

func application(_ app: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken token: Data) {
    let hex = token.map { String(format: "%02x", $0) }.joined()
    Task { try? await api.registerDevice(token: hex, environment: .current) } // POST /v1/devices
}

func application(_ app: UIApplication, didFailToRegisterForRemoteNotificationsWithError error: Error) {
    // Log only; the app works without push.
}
```

The app re-registers on every launch, because APNs can rotate tokens. Registration is an upsert, so repeating it costs nothing.

A `UNUserNotificationCenterDelegate` handles two things: showing a banner while the app is in the foreground (`willPresent`, except for `deposit_credited`, which the in-app toast announces), and routing a tap to the right screen (`didReceive`) using the ids in the payload (`cabal_id`, `proposal_id`, `txn_id`).

### API

- `POST /v1/devices` (auth required) with body `{ "token": "<hex>", "environment": "sandbox" | "production" }`. Upserts on `token` and reassigns the row to the caller if another user registered that device before.
- `DELETE /v1/devices/{token}` on logout. It stops a signed-out phone from getting another user's pushes.

Both routes are in `api/openapi.yaml` and take an `Idempotency-Key` header like every mutating call ([Thin client](backend-platform.md#thin-client)). The upsert is already idempotent; the header keeps the contract uniform.

### Tables

```
device_tokens
  id             uuid
  user_id        uuid      -> users
  token          text      unique, hex APNs device token
  environment    text      sandbox | production
  created_at     timestamptz
  last_seen_at   timestamptz  bumped on each registration
  disabled_at    timestamptz? set when APNs reports the token dead
```

**Notification** and **NotificationBroadcast** are already listed in [data-model.md](data-model.md). This decision adds **DeviceTokens** to that list.

### Sending (Go)

`internal/platform/apns` wraps [`github.com/sideshow/apns2`](https://github.com/sideshow/apns2) as an anti-corruption layer, the same shape as `platform/chain`: APNs wire types never leave the package, and `apns.New` takes functional options and builds one circuit breaker per environment ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). `notify` sends through the `apns.Sender` port, which the worker binds to `*apns.Client`, so no module imports apns2. `testkit.FakeSender` implements the port with `Fail` and `FailOnce`, and the `verify-backend` fakes server answers as APNs at `/apns/3/device/{token}` ([Verification skill](backend-platform.md#verification-skill)).

```go
type Sender interface {
    Send(ctx context.Context, n Push) (Result, error)
}

authKey, err := token.AuthKeyFromBytes([]byte(cfg.APNs.KeyP8))
tok := &token.Token{AuthKey: authKey, KeyID: cfg.APNs.KeyID, TeamID: cfg.APNs.TeamID}
sandbox := apns2.NewTokenClient(tok).Development()
prod    := apns2.NewTokenClient(tok).Production()

res, err := client.Push(&apns2.Notification{
    DeviceToken: deviceToken,
    Topic:       cfg.APNs.Topic,
    CollapseID:  "trade-" + txnID,
    Payload: payload.NewPayload().
        AlertTitle("Trade filled").
        AlertBody("Your cabal bought $50.00 of Tesla").
        Custom("cabal_id", cabalID).
        Custom("txn_id", txnID),
})
```

The client picks sandbox or production per token from `device_tokens.environment`. Debug builds from Xcode get sandbox tokens, and those fail against the production host.

Response handling:

| APNs result | Action |
| --- | --- |
| `200` | Mark the row `delivered`, set `notifications.delivered_at` and append `notification.sent`. |
| `410` (`Unregistered` or `ExpiredToken`), `400 BadDeviceToken` | Set `device_tokens.disabled_at`, never retry that token. A row whose every token is dead, or that has none, ends `no_device`. |
| `429`, `5xx`, network error | Leave the row `pending` and return the retryable `apns_unavailable`, so `bus.Dispatch` naks on its backoff schedule. A send error comes back as the client returned it. The redelivered message writes no row twice and resends only the rows still `pending`. |
| `403` auth errors | Record the `event_deliveries` row under `apns_auth_failed` and return that code: `bus.Dispatch` terms the message to `DEADLETTER` and alerts. The key or env is misconfigured, so every push is failing. It wins over every other answer of the delivery. |
| Any other status, such as `400 BadTopic` or `413 PayloadTooLarge` | Keep the token and the `pending` row, and ack. The push itself is wrong, so resending cannot help. |

`apns.Classify` maps these rows to `Delivered`, `TokenDead`, `Retry`, `AuthFailed` and `Rejected`. A `429` carries `Retry-After` in whole seconds as `Result.RetryAfter`, capped at an hour.

Error kinds and their bus verdicts come from the one `errs` code table ([Errors](backend-platform.md#errors)). Each APNs result is logged once as a decision with the token's `user_id` and the result, never the token itself ([Logs as evidence](backend-platform.md#logs-as-evidence)). Fan-out to a user's device tokens runs on a worker pool of 32 sharing one HTTP/2 client ([Concurrency rules](backend-platform.md#concurrency-rules)).

apns2 brings its own HTTP/2 client, so the adapter is an exception to the rule that outbound HTTP goes through `platform/httpclient`. It still takes its deadline from `MONACO_TIMEOUT_APNS`, trips a `gobreaker` breaker after five consecutive failures, and adds the OTel client span by hand. With `APNS_BASE_URL` set (refused in production), both environments send to that URL through Go's default transport instead of apns2's. A loopback http base URL gets plain HTTP/1.1, which the fakes server needs because it does not speak h2. An https base URL may negotiate HTTP/2. Without a base URL, the TLS dial takes the send's context, so a send that opens a new connection still ends at `MONACO_TIMEOUT_APNS` and not at apns2's 20 s dial timeout. The URL must be https or a loopback host, because every request carries the provider token. The device token sits in the request path, so the adapter strips the URL from transport errors, and a token that is not hexadecimal, or a collapse id APNs would refuse, is answered with the matching local `400` instead of being sent. In local and test with no `APNS_KEY_P8`, the worker binds `apns.NoopSender`, which answers `200` and logs `apns.noop_send` with the user id only.

### Payload

Every push carries a presentation block next to its copy: a thread, a category, an interruption level and who the push is from. The extension that draws the picture (a notification service extension that donates an `INSendMessageIntent`) and the action buttons read these fields. They are stored on the `notifications` row (`thread_id`, `category`, `interruption_level` and a `presentation` jsonb holding the sender and the group), so a resend after a crash sends exactly what the first attempt would have. A row from before the migration has NULLs and sends as it always did, with no `thread-id`, no `sound` and no sender keys.

```json
{
  "aps": {
    "alert": {"title": "...", "body": "..."},
    "thread-id": "cabal-<cabal id>",
    "category": "VOTE",
    "mutable-content": 1,
    "interruption-level": "time-sensitive",
    "sound": "default"
  },
  "kind": "...", "cabal_id": "...", "proposal_id": "...",
  "sender_kind": "person" | "cabal" | "app",
  "sender_id": "<user or cabal id>",
  "sender_name": "Mia",
  "sender_image_url": "https://.../avatars/...",
  "group_id": "<cabal id>", "group_name": "AC Capital", "group_image_url": "https://..."
}
```

- **Thread.** `aps.thread-id` is `cabal-<id>` for every push about a cabal (proposal, reminder, deciding vote, result, trade, pause, chat, access), `user-<id>` for a person (a new follower, a reply on a public post) and for the follower digest, `ledger` for the weekly Ledger, and `account` for deposit, nudge and test. iOS groups by it.
- **Category.** `aps.category` is empty by default. The vote reminder sets `VOTE`, the deciding-vote push sets `VOTE_DECIDING`, and the Ledger sets `LEDGER`, in the tickets that own that copy.
- **Interruption level.** `time-sensitive` only for the vote reminder and the deciding-vote push, which have a deadline. Everything else is `active`. The sender refuses any level outside `passive`, `active` and `time-sensitive` locally with `400 BadInterruptionLevel`, and never sends it.
- **Sender.** The person who acted (proposer, requester, poster, follower) or the cabal (approval, result, trade, pause). `app` is for the Ledger. Deposit, nudge and test pushes name no sender and send no sender keys. Image URLs are the public `avatars` bucket URL of the person's photo or the cabal's `PictureURL`; an empty one is left out and the extension falls back to initials and the tint.
- **Mutable content.** `aps.mutable-content` is `1` whenever a sender image or a group is present, so the extension runs. Account pushes do not set it.
- **Sound.** Every push with a presentation sets `aps.sound` to `default`.
- **Reserved keys.** `sender_kind`, `sender_id`, `sender_name`, `sender_image_url`, `group_id`, `group_name` and `group_image_url` belong to the presentation. A kind's own `Data` may not set one: the write fails with `invalid_input` and no row is written. `Data` may not set `aps` either.

In Go, a kind sets `Message.Presentation` with `ForCabal(view, sender)`, `ForPerson(card)` or `ForAccount()`, and `CabalSender(view)` or `PersonSender(card)` build the sender. A kind that adds a reply or mention push must set one.

Sample payloads for the simulator are `apps/mobile/PushFixtures/proposal_created_grouped.apns` (person-led, in a cabal thread) and `cabal_paused_grouped.apns` (cabal-led). `proposal_created.apns`, `proposal_result_filled.apns` and `proposal_result_failed.apns` show the vote pushes, and `vote_reminder.apns`, `vote_deciding.apns` and `close_reminder.apns` the reminder, the deciding vote and the opt-in close reminder; drop one with `xcrun simctl push "$UDID" xyz.monacolabs.app <file>`.

### Actions

The app registers the categories on every launch, before it sets the notification delegate, so a cold start from an action finds them. `PushCategory.all` in `MonacoCore/Push/PushCategories.swift` is the one source of truth.

| Category id | Actions (id, title, SF Symbol, options) |
|---|---|
| `VOTE` | `VOTE_YES` Yes `checkmark` (foreground, authenticationRequired); `VOTE_NO` No `xmark` (foreground, authenticationRequired); `READ_THESIS` Read the thesis `bubble.left` (foreground) |
| `VOTE_DECIDING` | `VOTE_YES`, `VOTE_NO` as above |
| `LEDGER` | none (routing and analytics only) |

`PushAction.route(actionIdentifier:payload:)` maps Yes and No to `PushRoute.voteHold` (the hold sheet with the pick made), Read the thesis to the proposal screen, and a plain tap or an unknown action to `PushRoute.parse`. A missing or invalid `proposal_id` or `cabal_id` falls back to `PushRoute.parse`. When the proposal's hold sheet or screen is already showing, `willPresent` returns `[.list]` only.

**How a vote from a notification is authenticated.**
- No vote ever leaves the notification. Yes and No are `.foreground` actions with `.authenticationRequired`, so iOS unlocks the phone (Face ID or passcode) and opens the app before the app sees the choice.
- The app then opens the hold sheet with the side picked. The member holds to vote, and the vote goes out as every vote does: `POST /v1/proposals/{id}/votes` with the signed-in session's token and an `Idempotency-Key`.
- No token, signature or vote intent is put in a push payload, and no new backend auth path exists.
- With no session, `PushRouter` holds the route until sign-in. If the proposal closed meanwhile, the hold sheet shows it closed.

### Pictures on pushes

A push is led by a picture with the Monaco icon on its corner only when iOS treats it as a communication notification. `MonacoNotificationService`, a notification service extension embedded in the app (`apps/mobile/MonacoNotificationService`), does that for every push with `mutable-content: 1`:

- **Keys it reads.** `sender_kind`, `sender_id`, `sender_name`, `sender_image_url`, `group_id`, `group_name`, `group_image_url` and `aps.thread-id` ([Payload](#payload)). `PushSenderPayload` parses them with Foundation only. `sender_kind = app`, or a missing kind, id or name, returns nothing and the push is delivered as sent, so the Ledger and deposit pushes keep the plain app icon. For `sender_kind = cabal` the cabal is both the sender and the group.
- **Host allow-list.** An image URL is used only when it is `https` with no credentials and its host is in the extension's own `Info.plist` key `MonacoImageHosts`, a comma separated list filled from the build setting `MONACO_IMAGE_HOSTS` (`MONACO_IMAGE_HOSTS_local`, `_staging` and `_production` in `Config/Environments/*.xcconfig`, picked by `MONACO_ENVIRONMENT` beside the API base URL). They name the Supabase project that serves the public `avatars` bucket, which holds profile photos and cabal pictures. Any other URL is dropped and the push still leads with the sender's name.
- **Fetch.** The sender and group images load in parallel on an ephemeral `URLSession`, 5 s each, at most 1 MB each, `200` only and decodable by ImageIO.
- **Intent.** The extension builds an `INSendMessageIntent` whose sender is an `INPerson` with the image, whose group name is the cabal's name with its picture, and whose conversation identifier is the thread. It donates the interaction as incoming and returns `content.updating(from: intent)`. iOS then draws the picture, and groups the pushes under the cabal's name.
- **Fallback.** Any failure (a timeout, a 404, an oversized or broken image, a refused donation) delivers the push with its title and body unchanged, with whatever image did load. `serviceExtensionTimeWillExpire` delivers the best content so far. The extension logs ids and hosts under `xyz.monacolabs.app` / `push-service`, never a body.
- **App side.** `Monaco.entitlements` carries `com.apple.developer.usernotifications.communication` and `.time-sensitive`, and the generated app Info.plists (`scripts/ensure-ios-privy-config.sh`) carry `NSUserActivityTypes = [INSendMessageIntent]`. Both capabilities must be on the App ID `xyz.monacolabs.app` in the developer portal before a signed build can show the communication style on a device.
- **No module targets.** The extension links no Swift package, because an extension has a small memory budget and module targets depend on `MonacoAPI`. `PushSenderPayload.swift` is also a source of `MonacoTests`, which tests it.

### Flow: trade confirmed

1. The `trading` module's swap state machine moves the `swaps` row to `confirmed`. In the same `uow.Do` it appends `trade.confirmed` with `{swap_id, cabal_id, ...}` (flow 11; [trade-execution.md](trade-execution.md), step 3). The relay publishes it with `Nats-Msg-Id` set to the event id.
2. The `notify` consumer receives it through `bus.Dispatch`. Its handler is a `bus.HandleOwn` handler named `notify.<type>`, so it runs outside the delivery transaction and again on every redelivery.
3. Each kind registered for the event names its recipients, here the voters of the proposal that started the trade, from the `governance` ballots port and the cabal's members through the read-only query port the `cabal` module exports ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)). The handler drops the event's causing user (its `actor_id` when the actor is a user) and deleted or unknown users, then renders the rest. One `uow.Do` writes a NotificationBroadcast when there are two or more and one Notification per recipient, `pending` or, past a kind's daily cap, `batched` (see [What notifies](#what-notifies-mvp)). Rows are unique on `(source_event_id, user_id, kind)`, so a redelivery writes nothing twice.
4. After commit, the handler sends every `pending` row of the event to each active `device_tokens` row of its user, on a pool of at most 32 sends, then settles each row in its own `uow.Do` as in the table above. APNs is never called inside a transaction ([event-bus.md](event-bus.md#consumers-and-handlers)). A crash between send and `delivered_at` can send twice; `apns-collapse-id` makes the phone show one.
5. Each delivered Notification appends `notification.sent` (flow 24). No consumer reacts to it.
6. Last, the handler records its `event_deliveries` row. A `429` or `5xx` naks before that, so the redelivery resends only the rows still `pending`.

Copy follows [product.md](../product.md): "cabal", not group; a human-readable stock name, never a mint address; no "xStock" branding.

### Preferences

A member can turn two push categories off. A missing row means on, so nobody has to opt in.

| Category | Kinds |
| --- | --- |
| `decision_requests` | `proposal_created`, `proposal_result`, `vote_reminder`, `vote_deciding`, `close_reminder`, `cabal_paused`, `cabal_resumed` |
| `replies_and_mentions` | `comment_reply`, `post_reply`, `comment_mention`, `chat_mention`, `chat_thread_reply` |
| Always on | `deposit_credited`, `new_follower`, `notify.follow_digest`, `nudge`, `cabal_access_requested`, `cabal_access_approved`, `weekly_ledger`, `test` |

- `GET /v1/me/notification-preferences` returns `{decision_requests, replies_and_mentions}`. `PUT` replaces both in one `uow.Do`, appends no event and returns what it stored. A deleted account is unauthorized.
- The table is `notification_preferences (user_id, category, enabled, updated_at)`, keyed on `(user_id, category)`.
- `Notify.Handle` makes one `GetPreferences` read for the distinct recipients of the event, after `Pusher.eligible`. It still writes a row for a recipient whose category for the kind is off, in state `muted` (the `state` check allows `muted`), so [Catch up](#catch-up) lists the reply or result. `send` reads only `pending` rows, so a `muted` row is never pushed, and `CountKindSince` skips it, so it never counts toward a daily cap. An ineligible recipient (deleted, or the event's actor) still gets no row. A kind with no category is never muted. A table test fails when a kind `Name()` in `app/` is in no bucket.
- Daily caps and batching (`stateFor`) are unchanged for rows that are not `muted`. There are no per-cabal or per-kind switches and no quiet hours.

### Catch up

The bell on Home opens Catch up. It is a read over the `notifications` rows plus batched reads from four modules, and one small state table. No second copy of the notifications is kept.

**Sources**, each item keyed so one proposal is one row:

| Section | Source | Item key |
| --- | --- | --- |
| Waiting on you | the viewer's open proposals with no ballot, from `governance` `PendingVotes`, soonest close first | `proposal:<id>` |
| Replies and mentions | rows of `comment_reply`, `post_reply`, `comment_mention`, `chat_mention` and `chat_thread_reply` from the last 30 days, newest first | `comment:<id>` or `chat:<message id>` |
| Invites | pending cabal invites from `cabal` `PendingInvites`, newest first | `invite:<request id>` |
| Results | rows of `proposal_result` from the last 30 days, newest first | `proposal:<id>` |
| Cleared today | `catch_up_state` rows cleared since the request's `since`, drawn from their `snapshot` only | any of the above |

A kind sets `ItemKey` through the `ItemKeyed` interface beside `Kind`; `Notify` copies it to `notifications.item_key`, and a row without a key is never listed. A reply to a post that is not a proposal carries `item_kind`, `item_label` and `item_actor_name` in its `data`, which Catch up returns as `post`.

**State.** `catch_up_state (user_id, item_key, read_at, cleared_at, cleared_reason, kept_at, snapshot, updated_at)`. The commands and the `notify_catch_up` consumer below write it.

- An item is **active** when it has no `cleared_at`, or when a source row newer than `cleared_at` arrived. A result for a proposal you voted on earlier reopens it as unread.
- The exception is a result for a proposal **your** ballot decided (`proposals.decided_by` is the viewer). It does not reopen the item, since the toast already told you.
- An item is **unread** when its newest source row is newer than `read_at`.
- A comment the viewer cannot see (deleted, removed or by a blocked author) is neither listed nor counted.
- The **count** is the number of active items. It is the bell badge and the "6 to catch up".

**Routes.** `GET /v1/me/catch-up?since=<RFC 3339>` (`getMyCatchUp`) returns `{count, cabals, waiting, replies, invites, results, cleared}`. `since` is the start of the viewer's local day and bounds `cleared`; it defaults to 24 hours ago. `cabals` is every cabal the viewer belongs to by join date, even when every section is empty. `GET /v1/me/catch-up/count` (`getMyCatchUpCount`) returns `{count}`. Every figure is computed on the server; the app only formats it. The backend port `notify/port.CatchUp.Count` serves the same number to other readers.

**Cross-module reads**, one batched call each per request, whatever the item count: `governance.PendingVotes`, `governance.ProposalCards`, `social.CommentPreviews` (each comment and the one it answers), `social.ReplyCounts`, `cabal.PendingInvites`, `cabal.CabalsOf` and `Cabals`, `treasury.TopHoldings` with `market` `ByMints` for the invite's top holding, and `identity.UsersByID` for chat authors.

**Hint.** `Pusher.write` publishes `user.<id>.catch_up_changed` after commit for every recipient of a row with an `item_key`, muted rows included. Every write below publishes it too, after commit.

**Commands.** `POST /v1/me/catch-up/items/{key}/read`, `/clear` and `/keep` (`readCatchUpItem`, `clearCatchUpItem`, `keepCatchUpItem`) each need an `Idempotency-Key`, answer 204 and run in one `uow.Do`. A key that is not an active item of the caller is `not_found`.

- **Read** sets `read_at`. **Keep** sets `kept_at` and `read_at` and leaves the item active. **Clear** sets `cleared_at` with the reason `cleared` and stores the snapshot.
- Every write is an upsert guarded by `catch_up_state.updated_at <= excluded.updated_at`, so a redelivered or reordered event never moves state backwards.

**Snapshot rule.** A cleared item must still draw after its source is gone: a cast ballot leaves `PendingVotes`, and an accepted invite leaves `PendingInvites`. So each clear stores the item as it was when it was cleared, rendered through the same assembly as the read, and Cleared today draws from that snapshot alone.

**Automatic clears.** The durable `notify_catch_up` clears an item when the viewer acts elsewhere. Each handler records its delivery in `event_deliveries` in the same transaction as its write.

| Event | Effect | Reason |
| --- | --- | --- |
| `vote.cast` | clears `proposal:<id>` for the voter; the snapshot is the proposal card with the voter's choice | `voted` |
| `vote.retracted` | reopens `proposal:<id>` when it was cleared as `voted`, so an undone ballot is waiting again; a hand-cleared item stays cleared | none |
| `comment.created` | clears the author's `comment:` items whose notification `feed_item_id` is the event's `feed_object_id`; the snapshot is the item as rendered; `chat:` items are only cleared by hand | `replied` |
| `cabal.access_decided`, `direction = invite`, `decision = accepted` | clears `invite:<request id>` for the invitee; the snapshot comes from `cabal` `InvitesByID` and `treasury.TopHoldings` | `joined` |
| `proposal.passed`, `failed`, `expired`, `withdrawn`, `voided` | no state; publishes the hint to every member of the cabal | none |
| `cabal.access_requested`, `direction = invite` | no state; publishes the hint to the invitee | none |

A `vote.retracted` writes a row even when nothing was cleared, stamped with the retraction time. A `vote.cast` delivered after it carries an older time, so the guard skips it and the proposal stays waiting.

**Reaction chips.** `social.CommentPreviews` fills each preview's `reactions` and `reaction_count` from the same comment reaction summaries as the thread, in one extra read for the whole batch. Catch up returns them on `comment` as `reactions` (the `Reaction` schema of the feed) and `reaction_count`; an emoji with no count is absent.

**Retention.** The rows Catch up reads are kept for 30 days. A prune of `notifications` (#6200) must keep `comment_reply`, `post_reply`, `comment_mention`, `chat_mention`, `chat_thread_reply` and `proposal_result` rows that long.

### What notifies (MVP)

Push only for MVP (default 2026-09-27). The in-app list is [Catch up](#catch-up), a read over these rows; there is no app-icon badge, and the app never sets `badge` in the payload. The only per-user mute settings are the two [preferences](#preferences) categories. Nobody is pushed about their own action: the handler drops any recipient equal to the event's `actor_id`.

| Push | Subject | Recipients | Copy | Source |
| --- | --- | --- | --- | --- |
| Proposal created | `proposal.created` (flow 9) | Every member but the proposer, who is the actor. Category `VOTE` for a member in the proposal's frozen voter set (`governance` `Ballots`, which holds a row for an eligible member who has not voted), none for the others. A `rule` proposal pushes nobody. Led by the proposer's photo | "`<proposer>` proposed Buy NVDA $400" / "“`<thesis>`” The vote closes in 6h." Without a thesis: "The vote closes in 6h." A sell reads "Sell TSLA $900", the USDC quote with no "about" | copy decided 2026-10-26 (F launch), from the F launch frames in `docs/design/f/` (#6444) |
| Vote reminder | The `notify.vote_reminders` poller (flow 33), once a third of the voting window has passed: 2 h into 6 h, 8 h into 24 h, 56 h into 7 days | The members of the proposal's frozen voter set who have no ballot, are still members and did not get the deciding-vote push, while the proposal is open. A `rule` proposal pushes nobody. Category `VOTE`, `time-sensitive`, led by the cabal's picture | "3 votes left · ends in 4h" / "Buy NVDA $400: 4 yes, 1 no so far. Yours counts." One vote left reads "1 vote left · …". Rows are unique per proposal and member | copy and timing decided 2026-10-26 (F launch, founder decision Q33), from the F launch frames `19-notifications` and `14-lock-screen-launch` (#6454) |
| Deciding vote | `vote.cast` (flow 10), when the proposal is open, not expired, and one more Yes passes it | The same members as the reminder, led by the cabal. A `rule` proposal pushes nobody. Category `VOTE_DECIDING`, `time-sensitive` | "AC Capital" / "Yours decides it: Buy NVDA $400 stands at 4 yes, 1 no and needs one more yes. Closes in 4h." Rows are unique per proposal and member, however the tally moves afterwards | copy decided 2026-10-26 (F launch), from the F launch frame `15-last-vote` (#6454) |
| Close reminder | `proposal.close_reminder_due` (flow 10b), an hour before the vote closes, or at once when the member asked inside that hour | The member who asked for "Remind me at close", alone. Category `VOTE`, `time-sensitive`, led by the cabal's picture; "Decision requests" off mutes it | "Buy NVDA $400 closes in 1h" / "You asked for a reminder. 4 yes, 1 no so far. Yours counts." A sell reads "Sell TSLA 2.2 sh closes in 40m". One per member and proposal (the poller's `sent_at`); it shares the collapse id `proposal-<id>` with the vote reminder, so the later of the two replaces the earlier banner | the opt-in reminder of #6454's scope note, founder decision Q33 (#6538) |
| Proposal result: filled | `trade.confirmed` (flow 11), when a proposal started the trade | Its voters who are still members. Members who did not vote get nothing about the trade. The deciding voter gets it, because the fill is a system event. A cash out or agent trade pushes nobody. Led by the cabal | "NVDA passed 5–1" / "Bought 1.82 sh at $219.40. You voted Yes." A sell reads "Sold …" | copy decided 2026-10-26 (F launch), from the F launch frames in `docs/design/f/` (#6444) |
| Proposal result: fill failed | `trade.failed` or `trade.blocked` (flow 11), when a proposal started the trade | The same voters as a filled result | "NVDA passed 5–1" / "The buy didn't go through. No money moved. You voted Yes." A blocked sell reads "The shares are still in the pot." instead of "No money moved." | copy decided 2026-10-26 (F launch), from the F launch frames in `docs/design/f/` (#6444) |
| Proposal result: failed | `proposal.failed` (flow 10) | The proposal's voters who are still members, except the voter whose ballot failed it, who is the actor. A `rule` proposal pushes nobody | "TSLA failed 2–2" / "No trade. You voted No." | copy decided 2026-10-26 (F launch), from the F launch frames in `docs/design/f/` (#6444) |
| Proposal result: expired | `proposal.expired` (flow 10) | The proposal's voters who are still members. A proposal with no ballots, or a `rule` proposal, pushes nobody | "SpaceX closed 1–1" / "Not enough votes by the close. No trade. You voted Yes." | copy decided 2026-10-26 (F launch), from the F launch frames in `docs/design/f/` (#6444) |
| Deposit credited | `deposit.credited` (flow 5) | The depositor. A deposit under one cent pushes nobody | "Deposit received" / "$25.00 is in your account balance." | default 2026-09-27; copy proposed 2026-10-06 (#627), needs product sign-off |
| Reply to you | `comment.created` (flow 21) | The author of the comment that was replied to, unless either of them blocked the other | "`<display name>` replied to you" / "“`<comment text>`” On `<post owner>`'s `<post label>`." ("On your `<post label>`." when the post is the recipient's). A GIF-only reply reads "Sent a GIF", a poll "Asked: `<question>`". Collapse id `comment-<root comment id>` | F launch (#6547), from `19-notifications` |
| Reply to your post | `comment.created` (flow 21) | The actor of the thread's post (`proposal`, `result`, `trade`, `member_joined`; never `price_move` or `cabal_created`), unless they wrote the reply, `comment_reply` already reaches them, or either of them blocked the other | "`<display name>` replied to your `<sale, proposal, buy, join>`" / "“`<comment text>`”", with the same GIF and poll bodies. Collapse id `post-<thread feed object id>` | F launch (#6547) |
| Cabal access requested | `cabal.access_requested` (flow 3) with direction `request` | Every admin of the cabal, the creator included. An invite pushes nobody | "New request to join" / "`<display name>` wants to join `<cabal name>`" | decided 2026-10-07 (#3462), needs product sign-off |
| Cabal access approved | `cabal.access_decided` (flow 3) with direction `request` and decision `approved` | The requester. A denial, a revocation, an expiry and an accepted invite push nobody | "You're in `<cabal name>`" / "Your request was approved. Say hi." | decided 2026-10-07 (#3462), needs product sign-off |
| Cabal paused | `cabal.paused` (flow 8), appended by `funding` when the first pause reason opens ([deposits-withdrawals.md](deposits-withdrawals.md#pause)) | Every member. A global pause pushes nobody, because ops announces it outside push. The push never says why it paused | "`<cabal name>` is paused" / "Trading is on hold while we check something. Your money is safe; we'll tell you when it resumes." | decided 2026-09-27; copy proposed 2026-10-06 (#627), needs product sign-off |
| Cabal resumed | `cabal.resumed` (flow 8), appended by `funding` when the last pause reason closes | Every member. It shares the paused push's collapse id, so it replaces that push on the phone | "`<cabal name>` is back" / "Trading has resumed." | decided 2026-09-27; copy proposed 2026-10-06 (#627), needs product sign-off |
| New follower | `follow.created` (flow 20) | The followee, for the first 3 follows of a UTC day. Later follows are batched, below. A follow with source `referral` pushes nobody; attribution sends no push | "New follower" / "`<display name>` (@`<handle>`) followed you" | decided 2026-09-27; copy proposed 2026-10-06 (#628), needs product sign-off |
| Follower digest | The `batched` follows of the previous UTC day, pushed by the `notify.follow_digest` poller | Each followee who is not deleted and has a batched follow | "New followers" / "2 more people followed you". One follow reads "1 more person followed you" | decided 2026-09-27; copy proposed 2026-10-06 (#628), needs product sign-off |

The result is one kind, `proposal_result`, that fires once per outcome and carries `outcome = filled | fill_failed | failed | expired`. Rows are unique per source event, so a retried fill notifies the voters again: a failed attempt sends the fill-failed result and a later successful attempt sends the filled one. Both share the collapse id `proposal-<proposal id>` with the proposal push, so the fill replaces the failure banner and the proposal push on the phone. `proposal_passed`, `trade_filled`, `trade_failed` and `trade_blocked` are retired: `notify` no longer consumes `proposal.passed`, and rows written under those kinds stay as they are. The title and body strings live in `notify/app/copy.go`. A ticker is the catalog symbol without its trailing `x`, or the display name for a Pre-IPO asset ("a stock" when the catalog lacks it). The close time rounds to the minute, then shows as minutes under an hour, hours under 48 and days after. A thesis is cut at 120 runes with "…".
| Chat mention or reply in your thread | `chat.message_posted` (flow 22) | The users in its `mentioned_user_ids`, the thread's participants | "`<cabal name>`" / "`<display name>` mentioned you". A participant who was not mentioned reads "`<display name>` replied in a thread you're in" | decided 2026-09-27 ([chat.md](chat.md)); copy proposed 2026-10-06 (#678), needs product sign-off |
| Onboarding nudge to add a phone | `user.nudge_due` with `kind` `add_phone` ([auth.md](auth.md#nudges)) | The user, while the account is active and the `auth_state` is still `AWAITING_PHONE` | "Find your friends on Monaco" / "Add your number to see who you know." | decided 2026-09-27; copy proposed 2026-10-06 (#679), needs product sign-off |
| Onboarding nudge to link X | `user.nudge_due` with `kind` `link_x` | The user, while the account is active and the `auth_state` is still `AWAITING_SOCIALS` | "Connect X" / "Link X to find people you already follow." | decided 2026-09-27; copy proposed 2026-10-06 (#679), needs product sign-off |
| Weekly Ledger | `ranking.ledger_ready`, appended by the `ranking` Ledger poller on Sundays at 17:00 UTC once the member's Ledger for the week is stored | The member, while the account is active and the stored Ledger exists. Category `LEDGER`, thread `ledger`, sender `app`, collapse id `ledger-<week>`, always on | "The Ledger · Week 42" / "You voted 3 of 3. Best call: Sell AMZN, +4.2%." With resolved calls and no best call: "You voted 3 of 3. No call landed this week." With no resolved calls: "Your stake +1.5% this week." (a loss uses the minus sign U+2212), or "You voted 3 of 3." when the stake return is unknown | copy decided 2026-10-29 (#6525) |
| Test push | `notify.test_requested` (flow 24), appended by `monacoctl notify test --user <id>` | That user | "Monaco test" / "Push is working." | proposed 2026-10-06 (#592), needs product sign-off |

**Reply thread rule.** `comment_reply` and `post_reply` group by where the reply was posted. A reply in a cabal's discussion (`reply_rule = members`: a proposal, and so any cabal-shaped result or reply card) uses `thread-id` `cabal-<cabal id>`, led by the replier with the cabal as the group. A reply on any other post, even one that belongs to a cabal, uses `user-<replier id>` and the replier as sender, with no group. `comment_mention` follows the same grouping: a reply whose `@handle` names a reader of the thread pushes that person `{name} mentioned you` with `“{excerpt}” On {item_label}.`, and `post_reply` and `comment_reply` leave the mentioned person out, so they get the one mention. All three carry `kind`, `feed_item_id` (the thread's post), `comment_id`, `root_comment_id`, `proposal_id?`, `cabal_id?`, `cabal_name?`, `item_kind`, `item_label` and `item_actor_name`; `comment_reply` adds `quoted_excerpt` (the answered comment). Rows are keyed `comment:<comment id>`.

The two vote pushes share the collapse id `proposal-<proposal id>` with the proposal and result pushes, so the newest one replaces an older banner on the phone, and #6538's opt-in "Remind me at close" push replaces the member-wide reminder if that is still showing. They carry `proposal_id` and `cabal_id`, so a tap opens the proposal, and the Yes and No actions of their categories open the hold sheet. The deciding push reads the live tally from governance's `VoteState` read port, and `Need` comes from the proposal's threshold through the domain tally function. `vote.cast` carries no tally, and governance appends no event for this.

The deciding push must be one row per proposal and member, not one per vote. A kind that implements `Keyed` (`SourceKey(e)`) writes its rows under that key in place of the event id, so `notifications` stays unique on `(source_event_id, user_id, kind)` across different `vote.cast` events of one proposal: `vote_deciding` uses a UUIDv5 of `deciding:<proposal id>`. The actor rule still reads the real event, and a redelivery resends only `pending` rows. A later No, or a Yes that flips to No and back, pushes nothing more.

The `notify.vote_reminders` poller runs every minute and walks the open trade proposals in `(due_at, id)` order, where `due_at = created_at + (expires_at - created_at) / 3`, through a cursor stored in `notify_reminder_cursor`, 50 proposals a tick. For each one it writes the rows under a UUIDv5 of `reminder:<proposal id>` and moves the cursor to that proposal in the same unit of work, so a crash resends only `pending` rows and a finished proposal is never scanned again. A tick first resends the cursor proposal's `pending` rows, in case a crash fell between the commit and the send. A failure stops the batch at that proposal. A proposal created after a tick has a later `due_at` than that tick's cursor, so the cursor never skips one.

Dollar amounts in copy truncate to whole cents and carry no thousands separator. A stock reads as its catalog display name, never its issuer symbol, and as "a stock" when the catalog has no entry for the symbol. A trade push carries the swap id as `txn_id`, and its collapse id is `trade-<swap id>`. A proposal push carries the proposal id as `proposal_id` and the cabal id as `cabal_id`, so a tap opens the proposal. Its kinds, plus Trade blocked, share the collapse id `proposal-<proposal id>`, so "passed" replaces "created" and "didn't go through" replaces "passed" on a phone that still shows it. A proposer with no name, or a deleted one, reads "Someone". A follower push carries the follower's id as `user_id`, and its collapse id is `follow-<follower id>`, so a follow after an unfollow replaces the earlier push. The follower reads "@`<handle>`" when the display name is the handle, "`<display name>`" when there is no handle, and "Someone" when the card is missing, deleted or has no name. The digest's collapse id is `follow-digest`, so the newest digest replaces an older one that is still on the phone. A nudge push carries the user's own id as `user_id`, and its collapse id is `nudge-<user id>`, so a later nudge replaces an earlier one that is still on the phone. The user may finish the step between the poller's tick and the delivery, so `notify` reads the user's card when it handles the event. A user who is deleted, suspended or banned, or whose `auth_state` no longer fits the nudge's `kind`, gets nothing. The poller is the event's actor, so the pipeline's actor rule keeps the user.

An access push carries the cabal id as `cabal_id`, so a tap opens the cabal screen, and both kinds share the collapse id `access-<request id>`, so "You're in" replaces the creator's request push for the same request on a shared phone. A requester with no name, or a deleted one, reads "Someone".

A reply push goes to the author of the comment that was replied to. The app shows one level of nesting, so a reply to a reply pushes the author of that reply and not the author of the thread's first comment. A comment with no parent, a reply to a deleted comment and a reply to your own comment push nobody. The body is the comment's excerpt. `social` cuts the excerpt at 120 runes, counted as Unicode code points, and adds no ellipsis. `notify` adds "…" to an excerpt of 120 runes, because a comment of exactly 120 runes cannot be told from a cut one. A commenter with no name, or a deleted one, reads "Someone". The push carries the feed item's id as `feed_item_id`, and a comment on a proposal also carries `proposal_id` and `cabal_id`. Its collapse id is `comment-<parent comment id>`, the id of the thread's first comment, so a newer reply in the same thread replaces an older one that is still on the phone.

A chat push goes to the users in the message's `mentioned_user_ids` and to the participants of the thread it replies to. `social` fills both lists from the cabal's members at post time, and `notify` reads the members again when it handles the event, so a user who left in between gets nothing. A user in both lists gets one push, the mention. Rows are unique on `(source_event_id, user_id, kind)`, so the thread kind leaves out the mentioned users to avoid a second row. The poster is the event's actor, so the pipeline's actor rule leaves the poster out. The title is the cabal's name. The body names the poster, or reads "Someone" when the poster has no name or is deleted, and never carries the message text. The push carries `cabal_id` and `message_id`, plus `parent_id` for a message in a thread, and a tap on either kind opens that thread. Its collapse id is `chat-<parent message id>` for a reply and `chat-<message id>` otherwise, so a newer push about a thread replaces an older one that is still on the phone.

No other event pushes, so the RFC's [flows table](backend-platform.md#flows) lists `notify` as a consumer only of the events in this table. Referral attribution and qualification, price moves, funding, cash outs, withdrawals, agent lifecycle and admin actions send no push. Unfollows never notify.

Follows are the only batched kind (default 2026-09-27). A kind caps itself by implementing `DailyCap()`, and the pipeline copies the number onto each of its notifications. `new_follower` returns 3. When `Pusher.write` finds that the user already has that many rows of the kind since 00:00 UTC, in any state, it writes the new row `batched` instead of `pending`. The send step reads only `pending` rows, so a batched row sends nothing, and the unique key keeps a redelivered event from adding a row. The cap is a constant. Add a `NOTIFY_FOLLOW_DAILY_CAP` setting when product wants to tune it without a deploy. No other kind is batched.

The `notify.follow_digest` poller runs every hour. Each tick takes the last UTC day that ended at least an hour ago, so the first tick after 01:00 UTC covers the day before. It pushes each followee who is not deleted one "N more people followed you" for that day's batched rows. The digest row's `source_event_id` is a UUIDv5 of the followee and the date, so a rerun writes nothing and resends only a row that is still `pending`. It writes no `event_deliveries` row, because that table references `events`. The poller has three known limits. It digests one day per tick, so an outage longer than a day drops the older digests. Its day scan filters `notifications` on `(kind, created_at)`, which has no index of its own. And the cap counts rows without a lock, so two workers that handle follows of one followee at the same moment can both push past it.

Onboarding nudges follow a fixed cadence (default 2026-10-03, #640). The `identity.nudges` poller runs every `IDENTITY_NUDGES_INTERVAL` (default `24h`) and appends one `user.nudge_due` for each user who meets all of these conditions:

- The user's `auth_state` is `AWAITING_PHONE` (kind `add_phone`) or `AWAITING_SOCIALS` (kind `link_x`).
- The account is `active` and not deleted.
- The `auth_state` has not changed for at least 24 hours.
- The user has no nudge yet, or the last one was at least 7 days ago.
- The user has had fewer than 3 nudges in the current state.

A user who stays stuck gets a nudge on days 1, 8 and 15, and then no more. Every `auth_state` change sets the count back to 0, so a user who unlinks later starts the cadence again. The event's `nudge_number` (1 to 3) says which nudge it is.

- The Simulator cannot reliably get a real remote device token. To test how a push looks and where a tap goes, drag a `.apns` file onto the Simulator, or run `xcrun simctl push <udid> xyz.monacolabs.app payload.json`.
- To test end to end (backend → APNs → phone), use a physical device running a debug build, which gets a sandbox token.
- `just test backend` never calls APNs. The push tests use the `testkit` fake `Sender`, and the flow 24 tests send through `*apns.Client` to an in-process fakes server. Every consumer test runs through the chaos dispatcher, so duplicate and reordered deliveries are tested, not assumed ([Keeping it deterministic](backend-platform.md#keeping-it-deterministic)).
- `monacoctl verify flow 24` drives the flow against real binaries. The worker sends through `*apns.Client` to the fakes server's `/apns` route, and the flow's outcomes cover a `429`, a `403` and a crash before commit.

## Rollout

The `notify` module lands in [Rollout](backend-platform.md#rollout) step 6, after the bus (step 2) and after the modules whose events it consumes. The old backend gets no push path; push waits for step 6 (default 2026-09-27). The earlier stopgap, calling the handler in a goroutine from the trade confirm path, is dropped: the RFC bans bare `go` statements in business code and makes NATS the only cross-module path.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Firebase Cloud Messaging | Adds the Firebase SDK to the app and a Google project, only to forward to APNs. Worth revisiting if an Android app happens. |
| OneSignal / other push SaaS | Same as FCM, plus user data sits with a vendor. Its segmentation and campaign tools overlap with NotificationBroadcast, which we own. |
| Certificate-based APNs auth (`.p12`) | Certificates expire every year and are per app. A `.p8` token key never expires and covers sandbox and production. |
| Send inline from the swap path | Can push for a rolled-back trade, or drop a push when APNs is down. See [Why](#why). |
| Push over the SSE stream only | `/v1/stream` ([SSE hub](backend-platform.md#sse-hub)) only reaches an app that is open, and its hints are droppable. Push is for when the app is closed. The two share the event source (the bus), not the transport. |

## TODO

- Write the push copy: title and body per kind in the [What notifies](#what-notifies-mvp) table, following [product.md](../product.md). This is writing work, not an open decision.

## Open questions

None.

Log: [log/notifications.md](log/notifications.md).
