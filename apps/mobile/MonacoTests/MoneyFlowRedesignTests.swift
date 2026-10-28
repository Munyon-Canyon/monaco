import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

private let ownAddress = "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU"

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
                WithdrawNetwork.solana.caveat,
                WithdrawNoteCopy.title,
                WithdrawCopy.arrivalNote(network: .solana, arrivalSeconds: 60),
                WithdrawCopy.addAddress,
                WithdrawCopy.invalidAddress,
                WithdrawCopy.ownDeposit,
                PlatformBalanceCard.pendingLine(micros: 50_000_000) ?? "",
            ]
        #expect(MainFlowCopyAudit.stringsAreClean(strings))
    }

    @Test func thePendingLineOnlyShowsWhenSomethingIsOnItsWay() {
        #expect(PlatformBalanceCard.pendingLine(micros: 0) == nil)
        #expect(PlatformBalanceCard.pendingLine(micros: 50_000_000) == "$50.00 pending")
    }
}
