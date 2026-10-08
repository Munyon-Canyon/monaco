import Testing

@testable import Monaco

@MainActor
struct AppNavigatorTransactionTests {
    private func cabal(_ id: String) -> AnyAppRoute { AnyAppRoute(CabalRoute(id: id)) }
    private func chat(_ id: String) -> AnyAppRoute { AnyAppRoute(ChatRoute(cabalID: id)) }
    private func txn(_ cabalID: String, _ id: String) -> AnyAppRoute {
        AnyAppRoute(TransactionRoute(cabalID: cabalID, transactionID: id))
    }

    @Test func aTransactionOpenedFromItsCabalSitsDirectlyAboveIt() {
        let navigator = AppNavigator()
        navigator.cabalsPath = [cabal("c1")]

        navigator.openTransaction(cabalID: "c1", transactionID: "t1", in: .cabals)

        #expect(navigator.cabalsPath == [cabal("c1"), txn("c1", "t1")])
    }

    @Test func screensAboveTheCabalAreDropped() {
        let navigator = AppNavigator()
        navigator.cabalsPath = [cabal("c1"), chat("c1")]

        navigator.openTransaction(cabalID: "c1", transactionID: "t1", in: .cabals)

        #expect(navigator.cabalsPath == [cabal("c1"), txn("c1", "t1")])
    }

    @Test func aRepeatOpenKeepsOneCabalAndOneTransaction() {
        let navigator = AppNavigator()
        navigator.cabalsPath = [cabal("c1")]

        navigator.openTransaction(cabalID: "c1", transactionID: "t1", in: .cabals)
        navigator.openTransaction(cabalID: "c1", transactionID: "t1", in: .cabals)

        #expect(navigator.cabalsPath == [cabal("c1"), txn("c1", "t1")])
    }

    @Test func anotherCabalOnTopIsDroppedAndTheCabalBelowIsReused() {
        let navigator = AppNavigator()
        navigator.cabalsPath = [cabal("c1"), cabal("c2")]

        navigator.openTransaction(cabalID: "c1", transactionID: "t1", in: .cabals)

        #expect(navigator.cabalsPath == [cabal("c1"), txn("c1", "t1")])
    }

    @Test func theTopmostCopyOfACabalOnThePathTwiceIsReused() {
        let navigator = AppNavigator()
        navigator.cabalsPath = [cabal("c1"), chat("c1"), cabal("c1")]

        navigator.openTransaction(cabalID: "c1", transactionID: "t1", in: .cabals)

        #expect(navigator.cabalsPath == [cabal("c1"), chat("c1"), cabal("c1"), txn("c1", "t1")])
    }

    @Test func aMissingCabalIsPushedFirst() {
        let navigator = AppNavigator()

        navigator.openTransaction(cabalID: "c1", transactionID: "t1", in: .home)

        #expect(navigator.homePath == [cabal("c1"), txn("c1", "t1")])
        #expect(navigator.cabalsPath.isEmpty)
        #expect(navigator.selectedTab == .home)
    }

    @Test func reusingTheCabalSelectsItsTab() {
        let navigator = AppNavigator()
        navigator.feedPath = [cabal("c1")]
        navigator.selectedTab = .home

        navigator.openTransaction(cabalID: "c1", transactionID: "t1", in: .feed)

        #expect(navigator.selectedTab == .feed)
        #expect(navigator.feedPath == [cabal("c1"), txn("c1", "t1")])
    }

    @Test func backFromTheTransactionLeavesTheCabalAsTheTopScreen() {
        let navigator = AppNavigator()
        navigator.cabalsPath = [cabal("c1"), chat("c1")]
        navigator.openTransaction(cabalID: "c1", transactionID: "t1", in: .cabals)

        navigator.cabalsPath.removeLast()

        #expect(navigator.cabalsPath == [cabal("c1")])
    }
}
