import Foundation
import MonacoCore
import Testing

@testable import Monaco

@Suite(.timeLimit(.minutes(1)))
struct AssetScrubHeaderTests {
    private static let utc = TimeZone.gmt
    private static let us = Locale(identifier: "en_US")
    private static let start: Int64 = 1_786_665_600
    private static let series = AssetChartSeries(
        range: .threeMonths,
        points: [
            .init(timestamp: start, priceUsdcMicros: 94_500_000),
            .init(timestamp: start + 86_400, priceUsdcMicros: 99_230_000),
            .init(timestamp: start + 2 * 86_400, priceUsdcMicros: 93_560_000),
        ])

    private static func header(_ index: Int?, chart: AssetChartSeries? = series) -> AssetScrubHeader? {
        AssetScrubHeader(chart: chart, index: index, locale: us, timeZone: utc)
    }

    @Test func scrubbedIndexShowsThatPointsPriceChangeAndDate() throws {
        let header = try #require(Self.header(1))

        #expect(header.price == UsdAmountFormatter.format(micros: 99_230_000))
        #expect(header.price == "$99.23")
        #expect(header.basisPoints == 500)
        #expect(PercentFormatter.format(basisPoints: try #require(header.basisPoints), signed: true) == "+5.00%")
        #expect(header.caption == "Sat, Aug 15")
    }

    @Test func scrubbingBelowTheFirstPointReportsALoss() throws {
        let header = try #require(Self.header(2))

        #expect(header.price == "$93.56")
        #expect(header.basisPoints == -99)
        #expect(header.caption == "Sun, Aug 16")
    }

    @Test func dayChartCaptionCarriesTheTime() throws {
        let day = AssetChartSeries(
            range: .oneDay,
            points: [
                .init(timestamp: Self.start + 14 * 3_600, priceUsdcMicros: 100_000_000),
                .init(timestamp: Self.start + 15 * 3_600, priceUsdcMicros: 101_000_000),
            ])

        let header = try #require(Self.header(1, chart: day))

        #expect(header.caption == "Fri 3:00\u{202F}PM")
        #expect(header.basisPoints == 100)
    }

    @Test func releaseOrAStaleIndexRestoresTheLiveHeader() {
        #expect(Self.header(nil) == nil)
        #expect(Self.header(3) == nil)
        #expect(Self.header(-1) == nil)
        #expect(Self.header(0, chart: nil) == nil)
    }
}
