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
   - `APNS_TOPIC=com.monaco.app` (the bundle id)

One key works for both sandbox and production, and for every app on the team.

### Device registration (iOS)

The app asks for permission at a moment the user understands, for example right after login or after their first trade, not at cold launch. Once the user allows it, the app registers for remote notifications and posts the token to the backend.

```swift
// AppDelegate, wired into the SwiftUI App with @UIApplicationDelegateAdaptor
func requestPushPermission() {
    UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { granted, _ in
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

A `UNUserNotificationCenterDelegate` handles two things: showing a banner while the app is in the foreground (`willPresent`), and routing a tap to the right screen (`didReceive`) using the ids in the payload (`cabal_id`, `proposal_id`, `txn_id`).

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

`internal/platform/apns` wraps [`github.com/sideshow/apns2`](https://github.com/sideshow/apns2) as an anti-corruption layer, the same shape as `platform/chain`: APNs wire types never leave the package, and `apns.New` takes functional options and builds one circuit breaker per environment ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). `notify/app` declares its own `Sender` port and the worker binds it to `*apns.Client`, so no module imports apns2. `testkit.FakeSender` implements the port with `Fail` and `FailOnce`, and the `verify-backend` fakes server answers as APNs at `/apns/3/device/{token}` ([Verification skill](backend-platform.md#verification-skill)).

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
| `200` | Set `notifications.delivered_at`. |
| `410` (`Unregistered` or `ExpiredToken`), `400 BadDeviceToken` | Set `device_tokens.disabled_at`, never retry that token. |
| `429`, `5xx`, network error | Leave `delivered_at` null and return a retryable `KindUnavailable` code, so `bus.Dispatch` naks with backoff. On `429` with `Retry-After`, the nak delay is that long. The redelivered message skips Notification rows already written and resends only undelivered ones. |
| `403` auth errors | Return a `KindInternal` code: `bus.Dispatch` terms the message to `DEADLETTER` and alerts. The key or env is misconfigured, so every push is failing. |
| Any other status, such as `400 BadTopic` or `413 PayloadTooLarge` | Never retry and keep the token. The push itself is wrong, so resending cannot help. |

`apns.Classify` maps these rows to `Delivered`, `TokenDead`, `Retry`, `AuthFailed` and `Rejected`. A `429` carries `Retry-After` in whole seconds as `Result.RetryAfter`, capped at an hour.

Error kinds and their bus verdicts come from the one `errs` code table ([Errors](backend-platform.md#errors)). Each APNs result is logged once as a decision with the token's `user_id` and the result, never the token itself ([Logs as evidence](backend-platform.md#logs-as-evidence)). Fan-out to a user's device tokens runs on a worker pool of 32 sharing one HTTP/2 client ([Concurrency rules](backend-platform.md#concurrency-rules)).

apns2 brings its own HTTP/2 client, so the adapter is an exception to the rule that outbound HTTP goes through `platform/httpclient`. It still takes its deadline from `MONACO_TIMEOUT_APNS`, trips a `gobreaker` breaker after five consecutive failures, and adds the OTel client span by hand. With `APNS_BASE_URL` set (refused in production), both environments send to that URL through Go's default transport instead of apns2's. A loopback http base URL gets plain HTTP/1.1, which the fakes server needs because it does not speak h2. An https base URL may negotiate HTTP/2. Without a base URL, the TLS dial takes the send's context, so a send that opens a new connection still ends at `MONACO_TIMEOUT_APNS` and not at apns2's 20 s dial timeout. The URL must be https or a loopback host, because every request carries the provider token. The device token sits in the request path, so the adapter strips the URL from transport errors, and a token that is not hexadecimal, or a collapse id APNs would refuse, is answered with the matching local `400` instead of being sent. In local and test with no `APNS_KEY_P8`, the worker binds `apns.NoopSender`, which answers `200` and logs `apns.noop_send` with the user id only.

### Flow: trade confirmed

1. The `trading` module's swap state machine moves the `swaps` row to `confirmed`. In the same `uow.Do` it appends `trade.confirmed` with `{txn_id, cabal_id, ...}` (flow 11; [trade-execution.md](trade-execution.md), step 3). The relay publishes it with `Nats-Msg-Id` set to the event id.
2. The `notify` consumer receives it through `bus.Dispatch`.
3. The handler loads the cabal's members through the read-only query port the `cabal` module exports ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)). It writes one NotificationBroadcast and one Notification per member, in the same transaction as the `event_deliveries` row, and commits. A redelivered event finds the `event_deliveries` row and does not notify twice.
4. After commit, for each Notification, the handler sends to every active `device_tokens` row of that user and records the result as in the table above. APNs is never called inside the transaction ([event-bus.md](event-bus.md#consumers-and-handlers)). A crash between send and `delivered_at` can send twice; `apns-collapse-id` makes the phone show one.
5. Each delivered Notification appends `notification.sent` (flow 24). No consumer reacts to it.

Copy follows [product.md](../product.md): "cabal", not group; a human-readable stock name, never a mint address; no "xStock" branding.

### What notifies (MVP)

Push only for MVP (default 2026-09-27). There is no in-app notification list and no app badge; the app never sets `badge` in the payload. There are no per-user mute settings, so `notification_preferences` is not built. Nobody is pushed about their own action: the handler drops any recipient equal to the event's `actor_id`.

| Push | Subject | Recipients | Source |
| --- | --- | --- | --- |
| Trade filled or failed | `trade.confirmed`, `trade.failed` (flow 11) | Cabal members | default 2026-09-27 |
| Proposal created | `proposal.created` (flow 9) | Cabal members | default 2026-09-27 |
| A proposal you voted on passed | `proposal.passed` (flow 10) | Its voters | default 2026-09-27 |
| Deposit credited | `deposit.credited` (flow 5) | The depositor | default 2026-09-27 |
| Reply to your comment | `comment.created` (flow 21) | The parent comment's author | default 2026-09-27 |
| Cabal paused or resumed | `cabal.paused`, `cabal.resumed` (flow 8), appended by `funding` when the first pause reason opens and the last closes ([deposits-withdrawals.md](deposits-withdrawals.md#pause)) | Every member | decided 2026-09-27 |
| New follower | `follow.created` (flow 20) | The followee | decided 2026-09-27; batched, below |
| Chat mention or reply in your thread | `chat.message_posted` (flow 22) | The users in its `mentioned_user_ids`, the thread's participants | decided 2026-09-27 ([chat.md](chat.md)) |
| Onboarding nudge | `user.nudge_due` ([auth.md](auth.md#nudges)) | The user | decided 2026-09-27 |

No other event pushes, so the RFC's [flows table](backend-platform.md#flows) lists `notify` as a consumer only of the events in this table. Referral attribution and qualification, price moves, funding, cash outs, withdrawals, agent lifecycle and admin actions send no push. Unfollows never notify.

Follows are the only batched kind (default 2026-09-27). The first 3 follows of the day push one by one; past that they collapse into one "5 people followed you" push per day. No other kind is batched, and only onboarding nudges are capped (below).

Onboarding nudges follow a fixed cadence (default 2026-10-03, #640). The `identity.nudges` poller runs every `IDENTITY_NUDGES_INTERVAL` (default `24h`) and appends one `user.nudge_due` for each user who meets all of these conditions:

- The user's `auth_state` is `AWAITING_PHONE` (kind `add_phone`) or `AWAITING_SOCIALS` (kind `link_x`).
- The account is `active` and not deleted.
- The `auth_state` has not changed for at least 24 hours.
- The user has no nudge yet, or the last one was at least 7 days ago.
- The user has had fewer than 3 nudges in the current state.

A user who stays stuck gets a nudge on days 1, 8 and 15, and then no more. Every `auth_state` change sets the count back to 0, so a user who unlinks later starts the cadence again. The event's `nudge_number` (1 to 3) says which nudge it is.

- The Simulator cannot reliably get a real remote device token. To test how a push looks and where a tap goes, drag a `.apns` file onto the Simulator, or run `xcrun simctl push <udid> com.monaco.app payload.json`.
- To test end to end (backend → APNs → phone), use a physical device running a debug build, which gets a sandbox token.
- `just test backend` uses the `testkit` fake `Sender` and never calls APNs. Every consumer test runs through the chaos dispatcher, so duplicate and reordered deliveries are tested, not assumed ([Keeping it deterministic](backend-platform.md#keeping-it-deterministic)).
- `monacoctl verify flow 24` drives the flow against real binaries with the fakes server standing in for APNs.

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
