import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct WithdrawRouteTests {
    private let signature = "signature-1"

    private func withdrawal(_ status: WithdrawalStatus) -> Withdrawal {
        Withdrawal(withdrawalID: "w-1", status: status, amountMicros: 2_000_000, txSignature: signature)
    }

    @Test func everyWithdrawLinkOpensTheWithdrawScreen() {
        #expect(WithdrawRoute().destination() is WithdrawView)
    }

    @Test func aConfirmedWithdrawalToastsWithASolscanLink() throws {
        let toast = try #require(WithdrawView.toast(for: .confirmed(withdrawal(.confirmed))))
        #expect(toast.message == "Withdrawal complete: $2.00")
        #expect(toast.isSuccess)
        #expect(toast.link?.url.absoluteString == "https://solscan.io/tx/\(signature)")
        #expect(toast.link?.title == "View on Solscan")
    }

    @Test func aFailedWithdrawalToastsThatNothingWasCharged() throws {
        let toast = try #require(WithdrawView.toast(for: .failed(code: "privy_unavailable")))
        #expect(toast.message == "Withdrawal didn't go through. Your balance wasn't charged.")
        #expect(!toast.isSuccess)
        #expect(toast.link == nil)
    }

    @Test func nothingSettledToastsNothing() {
        #expect(WithdrawView.toast(for: .submitting) == nil)
        #expect(WithdrawView.toast(for: .idle) == nil)
    }
}
