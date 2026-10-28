import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

private let ownAddress = "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU"
private let outsideAddress = "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"

/// The screens' types are main-actor isolated (the app's default), so their fixtures are too.
@MainActor
private enum Fixture {
    static func balance(_ micros: Int64, pending: Int64 = 0, depositAddress: String = ownAddress) -> AccountBalance {
        AccountBalance(
            availableMicros: micros, onChainMicros: micros + pending, inFlightMicros: pending,
            depositAddress: depositAddress, asOf: Date(timeIntervalSince1970: 1_759_579_200)
        )
    }
}

/// Withdraw: when Continue is live, and what the field says about an address.
@MainActor
struct WithdrawFormTests {
    @Test func continueNeedsAnAmountWithinTheBalanceAndAUsableAddress() {
        #expect(
            WithdrawForm(amountText: "100", destinationAddress: outsideAddress, balance: Fixture.balance(248_500_000))
                .canContinue)
        #expect(
            !WithdrawForm(amountText: "", destinationAddress: outsideAddress, balance: Fixture.balance(248_500_000))
                .canContinue)
        #expect(
            !WithdrawForm(amountText: "300", destinationAddress: outsideAddress, balance: Fixture.balance(248_500_000))
                .canContinue)
        #expect(
            !WithdrawForm(amountText: "100", destinationAddress: "", balance: Fixture.balance(248_500_000)).canContinue)
        #expect(
            !WithdrawForm(amountText: "100", destinationAddress: ownAddress, balance: Fixture.balance(248_500_000))
                .canContinue)
    }

    /// Nothing is said about an empty field; a pasted address that can't be used says why.
    @Test func theFieldOnlySpeaksUpAboutAnAddressItCannotUse() {
        #expect(WithdrawForm(amountText: "", destinationAddress: "", balance: Fixture.balance(1)).addressProblem == nil)
        #expect(
            WithdrawForm(amountText: "", destinationAddress: outsideAddress, balance: Fixture.balance(1)).addressProblem
                == nil)
        #expect(
            WithdrawForm(amountText: "", destinationAddress: "0xabc", balance: Fixture.balance(1)).addressProblem != nil
        )
        #expect(
            WithdrawForm(amountText: "", destinationAddress: ownAddress, balance: Fixture.balance(1)).addressProblem
                != nil)
    }

    @Test func anAmountOverTheBalanceTurnsTheHelperRed() {
        #expect(
            WithdrawForm(amountText: "300", destinationAddress: "", balance: Fixture.balance(248_500_000)).problem
                == "Not enough in your account balance.")
        #expect(
            WithdrawForm(amountText: "100", destinationAddress: "", balance: Fixture.balance(248_500_000)).problem
                == nil)
        #expect(
            !WithdrawForm(amountText: "5", destinationAddress: outsideAddress, balance: Fixture.balance(0))
                .canContinue)
    }

    @Test func anAmountOrAMaxUnderTheBackendsMinWithdrawalMicrosCannotContinue() {
        let balance = Fixture.balance(248_500_000)
        let under = WithdrawForm(amountText: "0.5", destinationAddress: outsideAddress, balance: balance)
        #expect(under.problem == WithdrawForm.belowMinimum)
        #expect(!under.canContinue)
        let one = WithdrawForm(amountText: "1", destinationAddress: outsideAddress, balance: balance)
        #expect(one.problem == nil)
        #expect(one.canContinue)
        let max = WithdrawForm(amountText: "0.5", destinationAddress: outsideAddress, balance: Fixture.balance(500_000))
        #expect(max.problem == WithdrawForm.belowMinimum)
        #expect(!max.canContinue)
        #expect(
            WithdrawForm(amountText: "0.5", destinationAddress: outsideAddress, balance: Fixture.balance(100_000))
                .problem == WithdrawForm.overBalance)
    }

    @Test func theHelperSaysWhatIsAvailable() {
        #expect(
            WithdrawForm(amountText: "", destinationAddress: "", balance: Fixture.balance(248_500_000)).balanceHelper
                == "$248.50 available")
    }

    @Test func theHelperRoundsDownSoItNeverPromisesMoreThanMaxFills() {
        #expect(
            WithdrawForm(amountText: "", destinationAddress: "", balance: Fixture.balance(2_998_175)).balanceHelper
                == "$2.99 available")
    }

    @Test func maxSendsTheWholeBalanceSubCentRemainderIncluded() {
        var amount = WithdrawAmount()
        amount.edit(to: "2.99")
        amount.tapMax()
        #expect(amount.micros(availableMicros: 2_998_175) == 2_998_175)
    }

    @Test func editingAfterMaxSendsWhatWasTyped() {
        var amount = WithdrawAmount()
        amount.edit(to: "2.99")
        amount.tapMax()
        amount.edit(to: "1")
        #expect(amount.micros(availableMicros: 2_998_175) == 1_000_000)
    }
}

/// Preset chips take two rows only at the accessibility sizes, each chip exactly once.
@MainActor
struct AmountEntryPresetRowTests {
    @Test func oneRowNormally() {
        #expect(AmountEntryText.presetRows(4, stacked: false) == [[0, 1, 2, 3]])
    }

    @Test func twoRowsAtTheAccessibilitySizesLongerHalfFirst() {
        #expect(AmountEntryText.presetRows(4, stacked: true) == [[0, 1], [2, 3]])
        #expect(AmountEntryText.presetRows(3, stacked: true) == [[0, 1], [2]])
    }

    @Test func twoChipsOrFewerNeverSplit() {
        #expect(AmountEntryText.presetRows(2, stacked: true) == [[0, 1]])
        #expect(AmountEntryText.presetRows(1, stacked: true) == [[0]])
        #expect(AmountEntryText.presetRows(0, stacked: true).isEmpty)
    }
}

/// The copy these screens added speaks the product's words, not the backend's.
@MainActor
struct MoneyFlowCopyTests {
    @Test func theNewCopyPassesTheMainFlowAudit() {
        let strings =
            DepositContent.steps + [
                DepositAddressCard.networkNote,
                CabalYield.fundNote,
                FundNoteCopy.share,
                FundSourceCopy.title,
                CabalCopy.noShareNotice,
                CabalCopy.fundAmountCaption,
                CabalCopy.fundFromCash,
                CabalCopy.fundToPot,
                WithdrawForm.caveat,
                WithdrawForm.overBalance,
                WithdrawForm.belowMinimum,
                WithdrawConfirmView.caveat,
                PlatformBalanceCard.pendingLine(micros: 50_000_000) ?? "",
            ]
        #expect(MainFlowCopyAudit.stringsAreClean(strings))
    }

    @Test func thePendingLineOnlyShowsWhenSomethingIsOnItsWay() {
        #expect(PlatformBalanceCard.pendingLine(micros: 0) == nil)
        #expect(PlatformBalanceCard.pendingLine(micros: 50_000_000) == "$50.00 pending")
    }
}
