import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct ProposeChainPickerTests {
    private func options() throws -> [ProposeChainOption] {
        let bestPrice = try #require(BestPrice.sample(now: Date(timeIntervalSince1970: 1_791_100_800)))
        return ProposeChainOption.options(bestPrice)
    }

    @Test func everyRowShowsItsOwnChainMark() throws {
        let rows = ProposeChainSheet.rows(try options(), selected: .best)

        #expect(
            rows.map(\.id) == [
                "propose-chain-best", "propose-chain-base", "propose-chain-solana", "propose-chain-ethereum",
                "propose-chain-monad",
            ])
        #expect(
            rows.map { ChainMark.imageName(for: $0.network) }
                == ["BaseMark", "BaseMark", "SolanaMark", "EthereumMark", "MonadMark"])
    }

    @Test func onlyTheChosenRowIsChecked() throws {
        let rows = ProposeChainSheet.rows(try options(), selected: .network(.solana))

        #expect(rows.map(\.isSelected) == [false, false, true, false, false])
        #expect(
            ProposeChainSheet.rows(try options(), selected: .best).map(\.isSelected) == [
                true, false, false, false, false,
            ])
    }

    @Test func rowCopySaysCabalAndShowsNoAddressOrIssuerName() throws {
        let rows = ProposeChainSheet.rows(try options(), selected: .best)

        #expect(rows.first?.caption == "The cabal buys wherever the price is best when the trade runs")
        #expect(rows.dropLast().last?.caption == "The cabal buys only on Ethereum")
        for row in rows {
            for text in [row.title, row.figure, row.caption] {
                #expect(!text.localizedCaseInsensitiveContains("xstock"))
                #expect(!text.contains("-"))
            }
        }
    }

    @Test func theLocationSheetListsEachPlaceTheCabalHoldsAndChecksTheOneChosen() {
        let holding = ProposeHolding(Components.Schemas.CabalPot.sampleMultichain.holdings[0])
        let onBase = holding.at(.base)

        let rows = ProposeLocationSheet.rows(onBase)

        #expect(rows.map(\.id) == ["propose-location-solana", "propose-location-base"])
        #expect(rows.map(\.title) == ["Solana", "Base"])
        #expect(rows.map(\.figure) == ["$249.98", "$547.95"])
        #expect(rows.map(\.caption) == ["0.7299 shares", "1.6 shares"])
        #expect(rows.map(\.isSelected) == [false, true])
        #expect(ProposeLocationSheet.rows(holding).map(\.isSelected) == [true, false])
        #expect(rows.map { ChainMark.imageName(for: $0.network.rawValue) } == ["SolanaMark", "BaseMark"])
    }
}
