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
    private func balance(_ micros: Int64) -> AccountBalance {
        AccountBalance(
            availableMicros: micros, onChainMicros: micros, inFlightMicros: 0, depositAddress: "x", asOf: Date())
    }

    @Test func fundNamesTheShortfallNotice() {
        let form = FundCabalForm(amountText: "750", balance: balance(500_000_000))
        #expect(form.noticeText == FundCabalForm.insufficientNotice)
        #expect(form.shortfallMicros == 250_000_000)
        #expect(FundCabalForm(amountText: "50", balance: balance(500_000_000)).noticeText == nil)
        #expect(FundCabalForm(amountText: "50", balance: balance(500_000_000)).shortfallMicros == nil)
    }

    @Test func cashLineAppendsWhatIsPending() {
        #expect(MoneyCashLine.text(availableMicros: 500_000_000, inFlightMicros: 0) == "Cash $500.00")
        #expect(
            MoneyCashLine.text(availableMicros: 500_000_000, inFlightMicros: 50_000_000)
                == "Cash $500.00 · $50.00 pending")
    }

    @Test func withdrawOutcomeSaysPendingUntilConfirmed() {
        #expect(WithdrawConfirmView.title(.pending) == "Withdrawal processing")
        #expect(WithdrawConfirmView.title(.completed) == "Withdrawal completed")
        #expect(WithdrawConfirmView.statusWord(.pending) == "Pending")
        #expect(WithdrawConfirmView.message(.failed) == WithdrawProgress.didNotGoThrough)
    }

    @Test func networkChooserNamesSolanaDirect() {
        let solana = DepositNetwork(id: DepositNetwork.solanaID, address: "a")
        #expect(DepositNetworksView.subtitle(for: solana) == "Direct USDC deposit")
    }
}
