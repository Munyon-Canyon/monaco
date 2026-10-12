import Foundation
import MonacoCore
import Testing

@testable import Monaco

struct CurveReadoutLineTests {
    private static let start = Date(timeIntervalSince1970: 1_790_931_600)
    private static let utc = TimeZone.gmt
    private static let us = Locale(identifier: "en_US")

    private static func curve(step: TimeInterval) -> ValueCurve {
        ValueCurve(
            samples: [(1_000_000_000, 0), (1_014_000_000, 14_000_000), (1_002_000_000, 2_000_000)]
                .enumerated().map { index, pair in
                    .init(
                        at: start.addingTimeInterval(TimeInterval(index) * step), value: Int64(pair.0),
                        pnl: Int64(pair.1))
                })
    }

    @Test func holdingAPointReadsItsValueItsChangeAndTheTime() throws {
        let day = Self.curve(step: 1_800)
        let held = try #require(day.nearestIndex(to: Self.start.addingTimeInterval(1_750)))

        let text = CurveReadoutLine.text(
            for: day.readout(at: held), range: .oneDay, locale: Self.us, timeZone: Self.utc
        ).replacingOccurrences(of: "\u{202F}", with: " ")

        #expect(held == 1)
        #expect(text == "$1,014.00 · +$14.00 · Fri 9:30 AM")
    }

    @Test func aMultiDayRangeReadsTheDateInsteadOfTheClock() throws {
        let month = Self.curve(step: 86_400)

        let text = CurveReadoutLine.text(
            for: month.readout(at: 2), range: .oneMonth, locale: Self.us, timeZone: Self.utc)

        #expect(text.hasPrefix("$1,002.00 · +$2.00 · "))
        #expect(text.contains("Oct"))
    }

    @Test func releasingRestoresTheLastPointAndNothingHeldReadsBlank() {
        let day = Self.curve(step: 1_800)

        #expect(CurveReadoutLine.text(for: nil, range: .oneDay) == " ")
        #expect(day.readout()?.value == "$1,002.00")
    }

    @Test func theChartDrawsTheSamePointsTheReadoutIndexes() throws {
        let day = Self.curve(step: 1_800)

        #expect(day.chartBars.count == 3)
        for index in day.chartBars.indices {
            let readout = try #require(day.readout(at: index))
            #expect(readout.at == day.chartBars[index].date)
        }
    }
}
