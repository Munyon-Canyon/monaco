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

/// Fund this cabal: what the amount step says and allows against the balance it has.
@MainActor
struct FundCabalFormTests {
    @Test func theButtonReadsTheAmountOnceThereIsOne() {
        #expect(FundCabalForm(amountText: "", balance: Fixture.balance(248_500_000)).ctaTitle == "Add money")
        #expect(FundCabalForm(amountText: "0", balance: Fixture.balance(248_500_000)).ctaTitle == "Add money")
        #expect(FundCabalForm(amountText: "50", balance: Fixture.balance(248_500_000)).ctaTitle == "Add $50 to the pot")
    }

    @Test func maxReadsTheAmountWithCents() {
        let balance = Fixture.balance(248_500_000)
        let max = FundCabalForm(amountText: "", balance: balance).maxDollars.map(AmountEntryText.plain) ?? ""
        #expect(FundCabalForm(amountText: max, balance: balance).ctaTitle == "Add $248.50 to the pot")
        #expect(FundCabalForm(amountText: "500", balance: balance).ctaTitle == "Add $500 to the pot")
    }

    @Test func anAmountOverTheBalanceCannotBeSent() {
        #expect(FundCabalForm(amountText: "248.50", balance: Fixture.balance(248_500_000)).problem == nil)
        #expect(
            FundCabalForm(amountText: "248.51", balance: Fixture.balance(248_500_000)).problem
                == "Not enough in your account balance.")
        #expect(FundCabalForm(amountText: "", balance: Fixture.balance(248_500_000)).problem == nil)
        #expect(
            FundCabalForm(amountText: "10", balance: Fixture.balance(0, pending: 300_000_000)).problem
                == "Not enough in your account balance.")
        #expect(FundCabalForm(amountText: "10", balance: nil).problem == nil)
    }

    /// The helper under the figure says what there is to fund with, and what is already on
    /// its way to a cabal, rather than the old "From your account balance" with no figure.
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

    @Test func theHelperSaysWhatIsAvailable() {
        #expect(
            FundCabalForm(amountText: "", balance: Fixture.balance(248_500_000)).availability == "$248.50 available")
        #expect(
            FundCabalForm(amountText: "", balance: Fixture.balance(198_500_000, pending: 50_000_000)).availability
                == "$198.50 available · $50.00 funding"
        )
        #expect(
            FundCabalForm(amountText: "", balance: Fixture.balance(1_000_000_000, pending: 300_000_000)).availability
                == "$1,000.00 available · $300.00 funding"
        )
        #expect(FundCabalForm(amountText: "", balance: nil).availability == nil)
    }

    /// The line under the pad names the cabal once its name has loaded.
    @Test func theNoteNamesTheCabalWhenThereIsAChoice() {
        #expect(
            FundCabalForm.note(into: nil)
                == "The money leaves your account balance and joins the pot. Your slice grows by the same amount.")
        #expect(
            FundCabalForm.note(into: "Semis or bust")
                == "The money leaves your account balance and joins the Semis or bust pot. Your slice grows by the same amount."
        )
        #expect(FundCabalForm.note(into: "") == FundCabalForm.note(into: nil))
    }

    @Test func maxIsTheWholeBalanceAndNothingWhenEmpty() {
        #expect(
            FundCabalForm(amountText: "", balance: Fixture.balance(248_500_000)).maxDollars == Decimal(string: "248.5"))
        #expect(FundCabalForm(amountText: "", balance: Fixture.balance(0)).maxDollars == nil)
    }
}

/// Which state Fund this cabal is in. The amount pad and its button only ever show together.
@MainActor
struct FundCabalStageTests {
    @Test func aFirstLoadIsLoadingOrItsFailure() {
        #expect(FundCabalStage.resolve(state: .loading) == .loading)
        #expect(FundCabalStage.resolve(state: .failed(.transport(URLError(.notConnectedToInternet)))) == .failed)
    }

    @Test func anEmptyBalanceAsksForMoneyFirst() {
        #expect(FundCabalStage.resolve(state: .loaded(Fixture.balance(0))) == .needsMoney)
        #expect(!FundCabalStage.needsMoney.showsAmountEntry)
    }

    @Test func aBalanceShowsTheAmountPad() {
        let funded = Fixture.balance(248_500_000)
        let stage = FundCabalStage.resolve(state: .loaded(funded))
        #expect(stage == .amount(funded))
        #expect(stage.showsAmountEntry)
    }

    @Test func moneyInFlightStillShowsTheAmountPad() {
        let funding = Fixture.balance(0, pending: 300_000_000)
        let stage = FundCabalStage.resolve(state: .loaded(funding))
        #expect(stage == .amount(funding))
        #expect(stage.showsAmountEntry)
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

    @Test func theHelperSaysWhatIsAvailable() {
        #expect(
            WithdrawForm(amountText: "", destinationAddress: "", balance: Fixture.balance(248_500_000)).balanceHelper
                == "$248.50 available")
    }
}

/// The deposit address card's three states, from the balance read that carries the address.
@MainActor
struct DepositAddressCardContentTests {
    @Test func aLoadInFlightWinsThenAnAddressThenWhatWentWrong() {
        #expect(DepositAddressCard.Content.resolve(.idle) == .loading)
        #expect(DepositAddressCard.Content.resolve(.loading) == .loading)
        #expect(
            DepositAddressCard.Content.resolve(.loaded(Fixture.balance(248_500_000)))
                == .ready(ownAddress))
        #expect(
            DepositAddressCard.Content.resolve(.failed(.transport(URLError(.notConnectedToInternet))))
                == .unavailable("Couldn't load your deposit address."))
    }

    @Test func anAddressThatCannotTakeMoneyIsNeverOffered() {
        #expect(
            DepositAddressCard.Content.resolve(.loaded(Fixture.balance(248_500_000, depositAddress: "")))
                == .unavailable("Couldn't load your deposit address."))
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
                FundCabalForm.note(into: nil),
                FundCabalForm.note(into: "Weekend investors"),
                FundCabalForm.overBalance,
                FundCabalForm.comingSoon,
                WithdrawForm.caveat,
                WithdrawForm.overBalance,
                WithdrawConfirmView.caveat,
                PlatformBalanceCard.pendingLine(micros: 50_000_000) ?? "",
            ]
        #expect(MainFlowCopyAudit.stringsAreClean(strings))
    }

    @Test func thePendingLineOnlyShowsWhenSomethingIsOnItsWay() {
        #expect(PlatformBalanceCard.pendingLine(micros: 0) == nil)
        #expect(PlatformBalanceCard.pendingLine(micros: 50_000_000) == "$50.00 funding a cabal")
    }
}
