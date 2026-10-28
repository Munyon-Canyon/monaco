import Foundation
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct CardFundingScreenTests {
    @Test func onlyAConfirmedStateShowsAnOutcome() {
        #expect(CardFundingScreen.resolve(.idle) == .handoff(working: false))
        #expect(CardFundingScreen.resolve(.creating) == .handoff(working: true))
        #expect(CardFundingScreen.resolve(.browsing(sessionID: "s")) == .handoff(working: false))
        #expect(CardFundingScreen.resolve(.processing(sessionID: "s")) == .checking)
        #expect(CardFundingScreen.resolve(.done(sessionID: "s")) == .received)
    }

    @Test func aFailedStartStaysOnTheHandoff() {
        #expect(CardFundingScreen.resolve(.failed(sessionID: nil, message: "x")) == .handoff(working: false))
        #expect(DepositView.cardStartFailed(.failed(sessionID: nil, message: "x")))
        #expect(!DepositView.cardStartFailed(.failed(sessionID: "s", message: "x")))
    }

    @Test func aFailedSessionIsCancelledWithTheExistingCopy() {
        #expect(
            CardFundingScreen.resolve(.failed(sessionID: "s", message: "x"))
                == .cancelled(CardDepositState.didNotGoThrough))
    }
}

@MainActor
struct MoneyFlowV3CopyTests {
    @Test func cashLineAppendsWhatIsPending() {
        #expect(MoneyCashLine.text(availableMicros: 500_000_000, inFlightMicros: 0) == "Cash $500.00")
        #expect(
            MoneyCashLine.text(availableMicros: 500_000_000, inFlightMicros: 50_000_000)
                == "Cash $500.00 · $50.00 pending")
    }

    @Test func networkChooserNamesSolanaDirect() {
        let solana = DepositNetwork(id: DepositNetwork.solanaID, address: "a")
        #expect(DepositNetworksView.subtitle(for: solana) == "Direct USDC deposit")
    }
}
