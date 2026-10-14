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
}
