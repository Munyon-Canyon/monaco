import Foundation
import MonacoCore
import Testing

@testable import Monaco

struct CurveChangeLineTests {
    private static let start = Date(timeIntervalSince1970: 1_790_931_600)
    private static let utc = TimeZone.gmt
    private static let us = Locale(identifier: "en_US")

    private static let curve = ValueCurve(
        samples: [(1_000_000_000, 0), (0, 14_000), (1_000_000_000, 2_000_000)]
            .enumerated().map { index, pair in
                .init(
                    at: start.addingTimeInterval(TimeInterval(index) * 1_800), value: Int64(pair.0),
                    pnl: Int64(pair.1))
            })

    @Test func restingReadsTheChangeAndTheRangeWording() throws {
        let change = try #require(Self.curve.change(.pnl))

        let text = CurveChangeLine.text(change: change, range: .oneDay)

        #expect(text.hasPrefix("+$"))
        #expect(text.contains("(+"))
        #expect(text.hasSuffix("Past day"))
    }

    @Test func holdingAPointReadsTheChangeToItAndTheTime() throws {
        let change = try #require(Self.curve.change(.pnl, to: 1))
        let at = Self.start.addingTimeInterval(1_800)

        let text = CurveChangeLine.text(
            change: change, range: .oneDay, at: at, locale: Self.us, timeZone: Self.utc
        ).replacingOccurrences(of: "\u{202F}", with: " ")

        #expect(text.contains("· Fri 9:30 AM"))
        #expect(!text.contains("Past day"))
    }

    @Test func aSubCentChangeReadsThePercentAlone() throws {
        let curve = ValueCurve(
            samples: [(3_000_000, 0), (3_000_000, 3_000)].enumerated().map { index, pair in
                .init(
                    at: Self.start.addingTimeInterval(TimeInterval(index) * 1_800), value: Int64(pair.0),
                    pnl: Int64(pair.1))
            })
        let change = try #require(curve.change(.pnl))

        let text = CurveChangeLine.text(change: change, range: .oneDay)

        #expect(text == "+0.10% Past day")
        #expect(!text.contains("$"))
    }

    @Test func aMissingChangeReadsBlank() {
        #expect(CurveChangeLine.text(change: nil, range: .oneDay) == " ")
    }
}
