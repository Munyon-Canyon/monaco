# Add a mobile route, deep link, tab or section

The app has one composition root, `Shell/AppEnvironment.swift`, built once in `MonacoApp.swift`
and read everywhere else with `@Environment(AppEnvironment.self)`. A feature never constructs
`APIClient`, `HintStream` or a tab's navigation itself. This page is the recipe for wiring a new
screen into that shell without editing a file another ticket owns. The rules live in #942.

## The unmigrated-screen rule

A screen, route or tab with nothing on the new backend yet renders `Shell/NotMigratedView.swift`:

```swift
NotMigratedView(screen: "Feed")
```

A rewire "lights up" its part by replacing that placeholder in its own file only: a tab file, a
route file or a section slot file. The legacy view stays in the tree, unreferenced from the tab
roots, until the rewire that owns it deletes it.

## Add a route

Declare the route type in the feature folder that owns the destination, conforming to
`Shell/AppRoute.swift`:

```swift
nonisolated struct CabalRoute: AppRoute {
    let id: String

    @MainActor func destination() -> some View {
        CabalScreen(cabalID: id)
    }
}
```

Mark the type `nonisolated` and `destination()` `@MainActor`. The app target defaults to main-actor
isolation, and a main-actor route type cannot satisfy the `Hashable` and `Sendable` requirements
that `AppRoute` declares.

Open it from anywhere that holds `AppEnvironment`:

```swift
environment.navigator.open(CabalRoute(id: cabalID), in: .cabals)
```

`AppNavigator.open(_:in:)` selects the tab and appends to that tab's path only; every other tab's
path is untouched. Each tab's `NavigationStack` already registers one
`.navigationDestination(for: AnyAppRoute.self)` in `Shell/MainTabView.swift`, so a new route type
needs no change there.

A route whose destination screen does not exist yet is a stub: its `destination()` returns
`NotMigratedView(screen:)` until the owning ticket replaces it in the same file. See
[Route stubs](#route-stubs) for the ones #942 declares ahead of their screens.

## Add a deep-link handler

`Shell/DeepLinkRouter.swift` asks each type in `DeepLinkRouter.handlers` for a route in array
order and opens the first match:

```swift
protocol DeepLinkHandler {
    static func route(for url: URL) -> (any AppRoute, MainTab)?
}
```

`MonacoApp.swift` calls `DeepLinkRouter.handle(_:navigator:)` from both `.onOpenURL` and
`.onContinueUserActivity(NSUserActivityTypeBrowsingWeb)`, so a universal link and a custom scheme
both resolve the same way. The `monaco` scheme is registered by #946 (`CFBundleURLTypes`); until
it lands only universal links reach the app.

Add a handler by creating one file in your feature folder and adding it to
`DeepLinkRouter.handlers`:

```swift
enum DepositDeepLink: DeepLinkHandler {
    static func route(for url: URL) -> (any AppRoute, MainTab)? {
        nil
    }
}
```

The handler returns `nil` until its ticket fills in the real match. Filling it in never touches
`DeepLinkRouter.swift` itself; the handler's own file is the only edit. #942 declares two stubs
ahead of their tickets:

| Handler | File | Owner |
| --- | --- | --- |
| `DepositDeepLink` | `Features/Deposit/DepositDeepLink.swift` | #650 |
| `ReferralDeepLink` | `Features/Referrals/ReferralDeepLink.swift` | #707 |

## Add a tab root

`Shell/MainTab.swift` lists all five product tabs: `home, feed, cabals, stocks, profile`. Each
tab's label, icon, accessibility id and root view come from a `TabContent` conformance in the
feature's own file, for example `Features/Feed/FeedTab.swift`:

```swift
enum FeedTab: TabContent {
    static let title = "Feed"
    static let systemImage = "newspaper"
    static let accessibilityIdentifier = "tab-feed"

    static func root() -> some View {
        NotMigratedView(screen: "Feed")
    }
}
```

`Shell/MainTabView.swift` loops over `MainTab.allCases` and never names a feature view type. A
tab lights up by replacing its `root()` body in its own file.

## Add a section slot

What each slot shows, in which order, and who builds it is in the [screen map](../screens.md). This
section is how to wire one.

Some shared screens — Home, Profile, the Cabals tab, the cabal screen, a user's profile and
proposal detail — are split into sections so a ticket can own one part of a screen without
editing the file every other ticket owns. `Shell/ScreenSection.swift` (#942) declares:

```swift
protocol ScreenSection<Context> {
    associatedtype Context
    associatedtype Body: View
    static var isLive: Bool { get }
    @MainActor @ViewBuilder static func body(for context: Context) -> Body
}
```

`SectionStack<Context>` renders, in array order, only the slots whose `isLive` is `true`. Each
screen declares its ordered slot list once and takes it as an init parameter defaulting to that
list, so a slot lights up by setting `isLive = true` and writing its body in its own file — never
in the screen file. A slot that loads data owns its own model and makes its own request; slots do
not share the host screen's model. A screen with no live slot renders `NotMigratedView`. The
Profile screen also shows a Sign out button under its slots, so a member can sign out before any
slot is live.

Home, Profile and the Cabals tab pull to refresh. Each tab root puts a `ScreenRefresh` in the
environment and runs it from `.refreshable`. A slot that loads data registers its reload, keyed by
the slot, from `@Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?`:
`refresh?.register("balance") { await model.load() }`. A pull runs every registered reload at once.

A slot is one stub file next to its screen. The cabal slots live in `Features/Groups/CabalSlots/`:

```swift
enum CabalPotSlot: CabalSection {
    static let isLive = false

    static func body(for context: CabalContext) -> some View {
        EmptyView()
    }
}
```

The owning ticket sets `isLive = true` and writes the body in that file. It never edits the screen
file.

A new screen declares a refining protocol, a context type when it needs one, and its ordered slot
list. Swift cannot convert `[any CabalSection.Type]` to `[any ScreenSection<CabalContext>.Type]`
implicitly, so the screen converts the list with `erased`:

```swift
protocol CabalSection: ScreenSection where Context == CabalContext {}

struct CabalScreen: View {
    static let sections: [any CabalSection.Type] = [CabalHeaderSlot.self, CabalPotSlot.self]

    let context: CabalContext
    let sections: [any CabalSection.Type]

    var body: some View {
        SectionStack(context: context, sections: sections.map { $0.erased })
    }
}
```

### Slots

| Screen and context | Slot | Owner |
| --- | --- | --- |
| `Features/Home/HomeScreen.swift` (toolbar avatar opens the Profile tab) | `HomeNudgeSlot` | #644 |
| | `HomePortfolioSlot` | #660 |
| | `HomeBalanceSlot` | #610 |
| | `HomePendingVotesSlot` | #612 |
| | `HomeCabalsSlot` | #660 |
| | `HomePeopleBoardSlot` | #699 |
| `Features/Profile/ProfileScreen.swift` | `ProfileHeaderSlot` | #644 |
| | `ProfileFollowCountsSlot` | #620 |
| | `ProfileStatsSlot` | #2140 |
| | `ProfileBalanceSlot` | #610 |
| | `ProfileCabalsSlot` | #2140 |
| | `ProfileInviteSlot` | #682 |
| | `ProfileFindFriendsSlot` | #663 |
| | `ProfileSettingsSlot` | #2139 |
| `Features/Groups/CabalsTabScreen.swift` | `CabalsInvitesSlot` | #696 |
| | `CabalsListSlot` | #606 |
| | `CabalsJoinSlot` | #646 |
| | `CabalsValueChartSlot` | #660 |
| | `CabalsBoardSlot` | #699 |
| `Features/Groups/CabalScreen.swift`, `CabalContext(cabalID: String)`, body | `CabalHeaderSlot` | #606 |
| | `CabalPotSlot` | #2137 |
| | `CabalValueChartSlot` | #660 |
| | `CabalSliceSlot` | #2137 |
| | `CabalPauseSlot` | #657 |
| | `CabalJoinSlot` | #646 |
| | `CabalActionsSlot` | #2134 |
| | `CabalProposalsSlot` | #612 |
| | `CabalHoldingsSlot` | #2137 |
| | `CabalAgentSlot` | #691 |
| | `CabalMemberBoardSlot` | #699 |
| | `CabalActivitySlot` | #654 |
| `CabalScreen` details sheet, "Cabal details" (toolbar `info.circle` button, hidden while no details slot is live) | `CabalInviteCodeSlot` | #646 |
| | `CabalInviteMemberSlot` | #696 |
| | `CabalRulesSlot` | #2135 |
| | `CabalTreasurySlot` | #651 |
| | `CabalEditSlot` | #647 |
| | `CabalLeaveSlot` | #697 |
| `Features/Profile/UserProfileScreen.swift`, `UserProfileContext(userID: String)` | `UserProfileHeaderSlot` | #620 |
| | `UserProfileSharedCabalsSlot` | #660 |
| `Features/Proposals/ProposalScreen.swift`, `ProposalContext(proposalID: String)` | `ProposalDetailSlot` | #612 |
| | `ProposalCommentsSlot` | #711 |

The cabal slot files live in `Features/Groups/CabalSlots/`; every other slot file sits next to its
screen.

### Route stubs

Routes to the section screens above, and routes a ticket opens before the screen it points to
exists, are declared ahead of time so an opener never waits on the screen's builder:

| Route | File | Owner | Opened by |
| --- | --- | --- | --- |
| `CabalRoute(id:)` | `Features/Groups/CabalRoute.swift` | #942 | many |
| `UserProfileRoute(userID:)` | `Features/Profile/UserProfileRoute.swift` | #942 | many |
| `ProposalRoute(proposalID:)` | `Features/Proposals/ProposalRoute.swift` | #942 | many |
| `ProposeRoute(cabalID:)` | `Features/Proposals/ProposeRoute.swift` | #613 | #612, #2134 |
| `TransactionRoute(cabalID:transactionID:)` | `Features/Groups/TransactionRoute.swift` | #654 | #671, #705 |
| `AssetRoute(symbol:)` | `Features/Assets/AssetRoute.swift` | #577 | #671 |
| `DepositRoute(prefillMicros: Int64? = nil, cabalID: String? = nil)` | `Features/Deposit/DepositRoute.swift` | #610, then #650 | #610, #651, #682 |
| `WithdrawRoute()` | `Features/Settings/WithdrawRoute.swift` | #652 | #610, #643 |
| `CashOutRoute(cabalID:)` | `Features/Redeem/CashOutRoute.swift` | #657 | #697, #2134 |
| `HandleEditRoute()` | `Features/Onboarding/HandleEditRoute.swift` | #693 | #644 |
| `DeleteAccountRoute()` | `Features/Settings/DeleteAccountRoute.swift` | #695 | #643, #695 |
| `JoinRoute()` | `Features/Groups/JoinRoute.swift` | #646 | #606 |
| `FundRoute(cabalID:)` | `Features/Deposit/FundRoute.swift` | #651 | #2134 |
| `ChatRoute(cabalID:)` | `Features/Groups/ChatRoute.swift` | #676 | #2134 |
| `SettingsRoute()` | `Features/Settings/SettingsRoute.swift` | #2139 | #2139 |
| `AccountActivityRoute()` | `Features/Settings/AccountActivityRoute.swift` | #2138 | #2139 |
| `AgentRoute(cabalID:)` | `Features/Groups/AgentRoute.swift` | #691 | #691 |
| `ProposeFromAssetRoute(symbol:kind:)` | `Features/Proposals/ProposeFromAssetRoute.swift` | #613 | #577 |

Ids are `String`s: the generated types these screens read do not exist yet.
