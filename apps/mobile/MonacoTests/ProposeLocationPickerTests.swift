import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct ProposeLocationPickerTests {
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
