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

    @Test func aProposalPushOpensItsCabalThenTheProposalOnTheCabalsTab() {
        let (router, navigator) = open(sessionOpen: true)

        router.handle(.proposal(proposalID: proposal, cabalID: cabal))

        #expect(navigator.selectedTab == .cabals)
        #expect(
            navigator.cabalsPath == [
                AnyAppRoute(CabalRoute(id: cabal)), AnyAppRoute(ProposalRoute(proposalID: proposal)),
            ]
        )
        #expect(untouched(navigator, except: .cabals))
    }

    @Test func aTransactionPushOpensItsCabalThenTheTransactionOnTheCabalsTab() {
        let (router, navigator) = open(sessionOpen: true)

        router.handle(.transaction(txnID: txn, cabalID: cabal))

        #expect(navigator.selectedTab == .cabals)
        #expect(
            navigator.cabalsPath == [
                AnyAppRoute(CabalRoute(id: cabal)),
                AnyAppRoute(TransactionRoute(cabalID: cabal, transactionID: txn)),
            ]
        )
        #expect(untouched(navigator, except: .cabals))
    }

    @Test func aChatPushOpensItsCabalThenTheChatOnTheCabalsTab() {
        let (router, navigator) = open(sessionOpen: true)

        router.handle(.chat(cabalID: cabal))

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: cabal)), AnyAppRoute(ChatRoute(cabalID: cabal))])
        #expect(untouched(navigator, except: .cabals))
    }

    @Test func aCabalPushOpensTheCabalOnTheCabalsTab() {
        let (router, navigator) = open(sessionOpen: true)

        router.handle(.cabal(cabalID: cabal))

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: cabal))])
        #expect(untouched(navigator, except: .cabals))
    }

    @Test func aFollowerPushOpensTheirProfileOnTheHomeTab() {
        let (router, navigator) = open(sessionOpen: true)
        navigator.selectedTab = .profile

        router.handle(.userProfile(userID: user))

        #expect(navigator.selectedTab == .home)
        #expect(navigator.homePath == [AnyAppRoute(UserProfileRoute(userID: user))])
        #expect(untouched(navigator, except: .home))
    }

    @Test func aFeedItemPushOpensTheItemOnTheFeedTab() {
        let (router, navigator) = open(sessionOpen: true)

        router.handle(.feedItem(feedItemID: feedItem))

        #expect(navigator.selectedTab == .feed)
        #expect(navigator.feedPath == [AnyAppRoute(FeedItemRoute(itemID: feedItem))])
        #expect(untouched(navigator, except: .feed))
    }

    @Test func aHomePushSelectsHomeAndPopsItsStackToTheRoot() {
        let (router, navigator) = open(sessionOpen: true)
        navigator.open(UserProfileRoute(userID: user), in: .home)
        navigator.open(CabalRoute(id: cabal), in: .home)
        navigator.open(CabalRoute(id: cabal), in: .cabals)
        navigator.selectedTab = .profile

        router.handle(.home)

        #expect(navigator.selectedTab == .home)
        #expect(navigator.homePath.isEmpty)
        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: cabal))])
    }

    @Test func aPushOpensAScreenAboveWhateverTheTabAlreadyShows() {
        let (router, navigator) = open(sessionOpen: true)
        navigator.open(CabalRoute(id: proposal), in: .cabals)

        router.handle(.cabal(cabalID: cabal))

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

    private func openWithEnvironment(sessionOpen: Bool) -> (PushRouter, AppNavigator, AppEnvironment) {
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(),
            hints: FakeHintSource(),
            isAuthenticated: { sessionOpen },
            endAuthSession: {}
        )
        if sessionOpen { environment.viewer = Viewer(userID: user, handle: nil) }
        return (PushRouter(environment: environment), environment.navigator, environment)
    }

    private func untouched(_ navigator: AppNavigator, except tab: MainTab?) -> Bool {
        MainTab.allCases.filter { $0 != tab }.allSatisfy { navigator.path(for: $0).isEmpty }
    }
}
