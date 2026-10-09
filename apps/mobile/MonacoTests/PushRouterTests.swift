import MonacoCore
import SwiftUI
import Testing
import UserNotifications

@testable import Monaco

@MainActor
struct PushRouterTests {
    private let user = "0192e8a1-0001-7000-8000-000000000001"
    private let cabal = "0192e8a1-0002-7000-8000-000000000002"
    private let proposal = "0192e8a1-0003-7000-8000-000000000003"
    private let txn = "0192e8a1-0004-7000-8000-000000000004"
    private let feedItem = "0192e8a1-0005-7000-8000-000000000005"

    @Test func aProposalPushOpensItsCabalThenTheProposalOnTheCabalsTab() async {
        let (router, navigator) = open(sessionOpen: true)

        await tap(.proposal(proposalID: proposal, cabalID: cabal), on: router)

        #expect(navigator.selectedTab == .cabals)
        #expect(
            navigator.cabalsPath == [
                AnyAppRoute(CabalRoute(id: cabal)), AnyAppRoute(ProposalRoute(proposalID: proposal)),
            ]
        )
        #expect(untouched(navigator, except: .cabals))
    }

    @Test func aTransactionPushOpensItsCabalThenTheTransactionOnTheCabalsTab() async {
        let (router, navigator) = open(sessionOpen: true)

        await tap(.transaction(txnID: txn, cabalID: cabal), on: router)

        #expect(navigator.selectedTab == .cabals)
        #expect(
            navigator.cabalsPath == [
                AnyAppRoute(CabalRoute(id: cabal)),
                AnyAppRoute(TransactionRoute(cabalID: cabal, transactionID: txn)),
            ]
        )
        #expect(untouched(navigator, except: .cabals))
    }

    @Test func aChatPushOpensItsCabalThenTheChatOnTheCabalsTab() async {
        let (router, navigator) = open(sessionOpen: true)

        await tap(.chat(cabalID: cabal), on: router)

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: cabal)), AnyAppRoute(ChatRoute(cabalID: cabal))])
        #expect(untouched(navigator, except: .cabals))
    }

    @Test func aCabalPushOpensTheCabalOnTheCabalsTab() async {
        let (router, navigator) = open(sessionOpen: true)

        await tap(.cabal(cabalID: cabal), on: router)

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: cabal))])
        #expect(untouched(navigator, except: .cabals))
    }

    @Test func aFollowerPushOpensTheirProfileOnTheHomeTab() async {
        let (router, navigator) = open(sessionOpen: true)
        navigator.selectedTab = .profile

        await tap(.userProfile(userID: user), on: router)

        #expect(navigator.selectedTab == .home)
        #expect(navigator.homePath == [AnyAppRoute(UserProfileRoute(userID: user))])
        #expect(untouched(navigator, except: .home))
    }

    @Test func aFeedItemPushOpensTheItemOnTheFeedTab() async {
        let (router, navigator) = open(sessionOpen: true)

        await tap(.feedItem(feedItemID: feedItem), on: router)

        #expect(navigator.selectedTab == .feed)
        #expect(navigator.feedPath == [AnyAppRoute(FeedItemRoute(itemID: feedItem))])
        #expect(untouched(navigator, except: .feed))
    }

    @Test func aHomePushSelectsHomeAndPopsItsStackToTheRoot() async {
        let (router, navigator) = open(sessionOpen: true)
        navigator.open(UserProfileRoute(userID: user), in: .home)
        navigator.open(CabalRoute(id: cabal), in: .home)
        navigator.open(CabalRoute(id: cabal), in: .cabals)
        navigator.selectedTab = .profile

        await tap(.home, on: router)

        #expect(navigator.selectedTab == .home)
        #expect(navigator.homePath.isEmpty)
        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: cabal))])
    }

    @Test func aPushOpensAScreenAboveWhateverTheTabAlreadyShows() async {
        let (router, navigator) = open(sessionOpen: true)
        navigator.open(CabalRoute(id: proposal), in: .cabals)

        await tap(.cabal(cabalID: cabal), on: router)

        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: proposal)), AnyAppRoute(CabalRoute(id: cabal))])
    }

    @Test(.timeLimit(.minutes(1)))
    func aTapBeforeTheSessionOpensWaitsForItAndThenOpensItsScreen() async {
        let (router, navigator, environment) = openWithEnvironment(sessionOpen: false)

        router.handle(.cabal(cabalID: cabal))

        #expect(navigator.selectedTab == .home)
        #expect(untouched(navigator, except: nil))
        environment.viewer = Viewer(userID: user, handle: nil)
        while navigator.cabalsPath.isEmpty { await Task.yield() }

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: cabal))])
    }

    @Test(.timeLimit(.minutes(1)))
    func theNewestTapWinsWhenSeveralWaitForTheSession() async {
        let (router, navigator, environment) = openWithEnvironment(sessionOpen: false)

        router.handle(.cabal(cabalID: proposal))
        router.handle(.feedItem(feedItemID: feedItem))
        router.handle(.cabal(cabalID: cabal))
        environment.viewer = Viewer(userID: user, handle: nil)
        while navigator.cabalsPath.isEmpty { await Task.yield() }

        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: cabal))])
        #expect(navigator.feedPath.isEmpty)
    }

    @Test(.timeLimit(.minutes(1)))
    func aTapAfterTheSessionOpensBeatsAnOlderTapStillWaiting() async {
        let (router, navigator, environment) = openWithEnvironment(sessionOpen: false)

        router.handle(.cabal(cabalID: proposal))
        environment.viewer = Viewer(userID: user, handle: nil)
        router.handle(.cabal(cabalID: cabal))
        for _ in 0..<10 { await Task.yield() }

        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: cabal))])
    }

    @Test(.timeLimit(.minutes(1)))
    func aPushWaitsForOpenSheetsToCloseBeforeItsScreenOpens() async {
        let closing = SheetClosing()
        let (router, navigator, _) = openWithEnvironment(sessionOpen: true, dismissPresented: closing.dismiss)

        router.handle(.proposal(proposalID: proposal, cabalID: cabal))
        while !closing.isWaiting { await Task.yield() }

        #expect(navigator.cabalsPath.isEmpty)
        #expect(navigator.selectedTab == .home)
        closing.finish()
        while navigator.cabalsPath.isEmpty { await Task.yield() }

        #expect(closing.count == 1)
        #expect(navigator.selectedTab == .cabals)
        #expect(
            navigator.cabalsPath == [
                AnyAppRoute(CabalRoute(id: cabal)), AnyAppRoute(ProposalRoute(proposalID: proposal)),
            ]
        )
    }

    @Test(.timeLimit(.minutes(1)))
    func aTapBeforeTheSessionOpensClosesSheetsOnlyOnceTheSessionIsOpen() async {
        let closing = SheetClosing()
        let (router, navigator, environment) = openWithEnvironment(
            sessionOpen: false, dismissPresented: closing.dismiss)

        router.handle(.cabal(cabalID: cabal))
        for _ in 0..<10 { await Task.yield() }

        #expect(closing.count == 0)
        environment.viewer = Viewer(userID: user, handle: nil)
        while !closing.isWaiting { await Task.yield() }

        #expect(navigator.cabalsPath.isEmpty)
        closing.finish()
        while navigator.cabalsPath.isEmpty { await Task.yield() }

        #expect(closing.count == 1)
    }

    @Test func finishingLaunchInstallsThePushCenterDelegate() {
        let (_, _, environment) = openWithEnvironment(sessionOpen: false)
        let center = UNUserNotificationCenter.current()
        let previous = center.delegate
        defer { center.delegate = previous }
        center.delegate = nil
        let appDelegate = AppDelegate()

        let finished = appDelegate.application(UIApplication.shared, didFinishLaunchingWithOptions: nil)

        #expect(finished)
        #expect(AppDelegate.environment === environment)
        #expect(center.delegate is PushCenterDelegate)
    }

    @Test(.timeLimit(.minutes(1)))
    func aTapThatWasAppliedIsNotReplayedOnTheNextSignIn() async {
        let (router, navigator, environment) = openWithEnvironment(sessionOpen: false)
        router.handle(.cabal(cabalID: cabal))
        environment.viewer = Viewer(userID: user, handle: nil)
        while navigator.cabalsPath.isEmpty { await Task.yield() }

        navigator.reset()
        environment.viewer = nil
        environment.viewer = Viewer(userID: user, handle: nil)
        for _ in 0..<10 { await Task.yield() }

        #expect(navigator.selectedTab == .home)
        #expect(untouched(navigator, except: nil))
    }

    private func open(sessionOpen: Bool) -> (PushRouter, AppNavigator) {
        let (router, navigator, _) = openWithEnvironment(sessionOpen: sessionOpen)
        return (router, navigator)
    }

    private func tap(_ route: PushRoute, on router: PushRouter) async {
        router.handle(route)
        for _ in 0..<10 { await Task.yield() }
    }

    private func openWithEnvironment(
        sessionOpen: Bool,
        dismissPresented: @escaping @MainActor () async -> Void = {}
    ) -> (PushRouter, AppNavigator, AppEnvironment) {
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(),
            hints: FakeHintSource(),
            isAuthenticated: { sessionOpen },
            endAuthSession: {}
        )
        if sessionOpen { environment.viewer = Viewer(userID: user, handle: nil) }
        let router = PushRouter(environment: environment, dismissPresented: dismissPresented)
        return (router, environment.navigator, environment)
    }

    private func untouched(_ navigator: AppNavigator, except tab: MainTab?) -> Bool {
        MainTab.allCases.filter { $0 != tab }.allSatisfy { navigator.path(for: $0).isEmpty }
    }
}

@MainActor
private final class SheetClosing {
    private var release: CheckedContinuation<Void, Never>?
    private(set) var count = 0

    var isWaiting: Bool { release != nil }

    func dismiss() async {
        count += 1
        await withCheckedContinuation { release = $0 }
    }

    func finish() {
        release?.resume()
        release = nil
    }
}
