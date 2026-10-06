import Foundation
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

/// The pot-mix bar's slices: largest stock first, cash last, nothing for a zero row.
struct PotMixBarTests {
    private func row(_ symbol: String, value: String) -> PotRowDTO {
        PotRowDTO(symbol: symbol, units: "1", markUsd: "1", valueUsd: value, dollarPnl: "+0.00", afterHours: nil)
    }

    @Test func stocksLeadByValueAndCashCloses() {
        let segments = PotMixBar.segments(for: [
            row("USDC", value: "50.00"),
            row("NVDAx", value: "150.00"),
            row("AAPLx", value: "300.00"),
        ])
        #expect(segments.map(\.symbol) == ["AAPL", "NVDA", "Cash"])
        #expect(segments.map(\.isCash) == [false, false, true])
        #expect(abs(segments[0].fraction - 0.6) < 0.0001)
        #expect(abs(segments[2].fraction - 0.1) < 0.0001)
    }

    @Test func zeroAndUnparseableRowsAreDropped() {
        let segments = PotMixBar.segments(for: [
            row("USDC", value: "0.00"),
            row("AAPLx", value: "not money"),
            row("TSLAx", value: "12.00"),
        ])
        #expect(segments.map(\.symbol) == ["TSLA"])
        #expect(segments[0].fraction == 1)
    }

    @Test func anEmptyPotHasNoBar() {
        #expect(PotMixBar.segments(for: []).isEmpty)
        #expect(PotMixBar.segments(for: [row("USDC", value: "0")]).isEmpty)
    }

    @Test func slicesUnderOnePercentSayLessThanOne() {
        #expect(PotMixBar.percent(0.004) == "<1%")
        #expect(PotMixBar.percent(0.5) == "50%")
    }
}

/// Custom faces do not synthesise weights, so a weight asked for in SwiftUI terms has to land
/// on a real face of the family.
struct MonacoTypefaceTests {
    @Test func everyWeightLandsOnARealFace() {
        #expect(MonacoTypeface.avenirNext(.regular) == "AvenirNext-Regular")
        #expect(MonacoTypeface.avenirNext(.medium) == "AvenirNext-Medium")
        #expect(MonacoTypeface.avenirNext(.semibold) == "AvenirNext-DemiBold")
        #expect(MonacoTypeface.avenirNext(.bold) == "AvenirNext-Bold")
        #expect(MonacoTypeface.avenirNext(.heavy) == "AvenirNext-Bold")
        #expect(MonacoTypeface.avenirNext(.light) == "AvenirNext-Regular")
    }

    /// The family ships with iOS; a missing face would fall back to a system font silently.
    @Test func theFacesExistOnThisRuntime() {
        for name in ["AvenirNext-Regular", "AvenirNext-Medium", "AvenirNext-DemiBold", "AvenirNext-Bold"] {
            #expect(UIFont(name: name, size: 17) != nil, "\(name) is not installed")
        }
    }

    /// Money lines up in a column because Avenir Next's lining figures are tabular by default.
    @Test func avenirNextFiguresAreTabular() {
        let font = UIFont(name: "AvenirNext-DemiBold", size: 40)!
        let narrow = ("1111.11" as NSString).size(withAttributes: [.font: font]).width
        let wide = ("8888.88" as NSString).size(withAttributes: [.font: font]).width
        #expect(abs(narrow - wide) < 0.01)
    }
}
