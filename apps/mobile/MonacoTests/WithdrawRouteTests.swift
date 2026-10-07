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

    @Test func maxShowsOneFlooredAmountOnConfirmAndToast() throws {
        let available: Int64 = 2_998_175
        var amount = WithdrawAmount()
        amount.tapMax()
        let label = try #require(amount.fullBalanceLabel(availableMicros: available))
        #expect(label == "$2.99 (full balance)")
        #expect(amount.micros(availableMicros: available) == available)
        let sent = Withdrawal(withdrawalID: "w-1", status: .confirmed, amountMicros: available, txSignature: signature)
        #expect(WithdrawView.message(for: .submitted(sent), fullBalanceLabel: label)?.contains(label) == true)
        let toast = try #require(WithdrawView.toast(for: .confirmed(sent), fullBalanceLabel: label))
        #expect(toast.message == "Withdrawal complete: \(label)")
    }

    @Test func typedAmountsKeepTheirPlainToast() throws {
        var amount = WithdrawAmount()
        amount.edit(to: "2")
        #expect(amount.fullBalanceLabel(availableMicros: 2_998_175) == nil)
    }
}
