import MonacoAPI
import XCTest

@testable import MonacoCore

extension Components.Schemas.ProposalDetail {
    fileprivate static func meta(now: Date) -> Self {
        var detail = filledMETA(now: now)
        detail.cabalName = "Degens"
        detail.tally = .init(yes: 4, no: 1, voters: 6, needed: 4)
        detail.proposer = .init(id: detail.proposerId, displayName: "Christelle", photoUrl: nil)
        return detail
    }
}

final class SharePassedBuilderTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_000_000)
    private let crest = ShareCrest(cabalID: "cabal-1", name: "Degens", pictureURL: nil)
    private let utc = TimeZone(identifier: "UTC") ?? .gmt

    private func chart() -> [ShareChartPoint] {
        (0..<6).map { (i: Int) -> ShareChartPoint in
            let offset: TimeInterval = TimeInterval(i * 3_600 - 40_000)
            let priceMicros: Int64 = Int64(700 + i) * 1_000_000
            return ShareChartPoint(time: now.addingTimeInterval(offset), priceMicros: priceMicros)
        }
    }

    private func build(
        _ proposal: Components.Schemas.ProposalDetail? = nil, eyes: ShareEyes = ShareEyes(pnlDollars: false)
    ) -> ShareCardSpec {
        SharePassedBuilder.build(
            proposal: proposal ?? .meta(now: now), crest: crest, proposerPhoto: nil, chart: chart(),
            code: "andres", eyes: eyes, timeZone: utc)
    }

    private func proposal(
        swap: Components.Schemas.ProposalDetail.SwapPayload?, status: Components.Schemas.ProposalStatus = .passed,
        kind: Components.Schemas.ProposalKind = .buy
    ) -> Components.Schemas.ProposalDetail {
        var detail = Components.Schemas.ProposalDetail.meta(now: now)
        detail.swap = swap
        detail.status = status
        detail.kind = kind
        if kind == .sell {
            detail.usdcMicros = nil
            detail.tokenAmount = 42_000_000
        }
        if swap?.status != .confirmed { detail.stats.sinceBps = nil }
        return detail
    }

    private func swap(
        _ status: Components.Schemas.ProposalDetail.SwapPayload.StatusPayload, price: Int64? = nil, at: Date? = nil
    ) -> Components.Schemas.ProposalDetail.SwapPayload {
        .init(
            swapId: "swap-1", status: status, failureCode: nil, failureMessage: nil, txSignature: nil, retryable: false,
            tokenDecimals: 8, priceMicros: price, filledAt: at)
    }

    func testTheFrameSpec() throws {
        let spec = build()

        XCTAssertEqual(spec.header.title, "Christelle called it")
        XCTAssertEqual(spec.header.subtitle, "Degens \u{00B7} Buy META $300 \u{00B7} passed 4\u{2013}1")
        guard case .photo(_, let name, let mark) = spec.header.mark else { return XCTFail("\(spec.header.mark)") }
        XCTAssertEqual(name, "Christelle")
        XCTAssertEqual(mark, crest)
        XCTAssertEqual(spec.tickerLine.symbol, "META")
        XCTAssertEqual(spec.tickerLine.detail, "0.42 sh")
        XCTAssertEqual(spec.figure.main, "+1.1%")
        XCTAssertEqual(spec.figure.context, "since the fill")
        XCTAssertNil(spec.figure.secondary)
        XCTAssertEqual(
            spec.stats,
            [
                .init(label: "Bought", value: "0.42 sh"), .init(label: "At", value: "$716.52"),
                .init(label: "Now", value: "$724.40"),
            ])
        XCTAssertEqual(spec.code, "ANDRES")
        XCTAssertEqual(spec.replayTitle, "Replay the call")
    }

    func testTheSealAndTheFillAreMarkedWithTheirCallouts() throws {
        var detail = Components.Schemas.ProposalDetail.meta(now: now)
        detail.passedPriceMicros = 709_800_000
        let markers = build(detail).chart.markers

        XCTAssertEqual(markers.map(\.kind), [.seal, .buy])
        XCTAssertEqual(markers[0].priceMicros, 709_800_000)
        XCTAssertEqual(markers[0].callout, "Passed \u{00B7} $709.80")
        XCTAssertEqual(markers[0].time, detail.stats.closedAt)
        XCTAssertEqual(markers[1].priceMicros, 716_520_000)
        XCTAssertEqual(markers[1].callout, "Filled \u{00B7} $716.52")
        XCTAssertEqual(markers[1].time, detail.swap?.filledAt)
    }

    func testPnlDollarsOnGivesTheCabalsDollarChange() {
        let spec = build(eyes: ShareEyes(pnlDollars: true))

        XCTAssertEqual(spec.figure.main, "+$3.31")
        XCTAssertEqual(spec.figure.secondary, "+1.1%")
        XCTAssertEqual(spec.figure.context, "since the fill")
    }

    func testTheDollarChangeIsIntegerMathAndKeepsItsSign() {
        XCTAssertEqual(
            SharePassedBuilder.dollarChange(atomics: 42_000_000, decimals: 8, from: 716_520_000, to: 724_400_000),
            3_309_600)
        XCTAssertEqual(
            SharePassedBuilder.dollarChange(atomics: 42_000_000, decimals: 8, from: 724_400_000, to: 716_520_000),
            -3_309_600)
        XCTAssertEqual(SharePassedBuilder.dollarChange(atomics: 42_000_000, decimals: 8, from: 5, to: 5), 0)
        XCTAssertNil(SharePassedBuilder.dollarChange(atomics: nil, decimals: 8, from: 1, to: 2))
        XCTAssertNil(SharePassedBuilder.dollarChange(atomics: 1, decimals: 40, from: 1, to: 2))
    }

    func testANullPassedPriceDropsTheSeal() {
        var detail = Components.Schemas.ProposalDetail.meta(now: now)
        detail.passedPriceMicros = nil
        let spec = build(detail)

        XCTAssertEqual(spec.chart.markers.map(\.kind), [.buy])
        XCTAssertEqual(spec.chart.callouts.count, 1)
    }

    func testPassedWithNoSwapIsATruePendingCard() {
        let spec = build(proposal(swap: nil))

        XCTAssertEqual(spec.figure.main, "Passed 4\u{2013}1")
        XCTAssertEqual(spec.figure.context, "buying now")
        XCTAssertFalse(spec.chart.markers.contains { $0.kind == .buy || $0.kind == .sell })
        XCTAssertEqual(spec.tickerLine.detail, "")
        assertNoZeroPrice(spec)
    }

    func testAPendingSwapIsTheSameAsNoSwap() {
        let spec = build(proposal(swap: swap(.submitted)))

        XCTAssertEqual(spec.figure.context, "buying now")
        XCTAssertFalse(spec.chart.markers.contains { $0.kind == .buy })
        XCTAssertEqual(spec.stats.map(\.label), ["Amount", "Passed at", "Now"])
        assertNoZeroPrice(spec)
    }

    func testAPendingSellSaysSellingNow() {
        XCTAssertEqual(build(proposal(swap: nil, kind: .sell)).figure.context, "selling now")
    }

    func testAFailedTradeSaysSoAndMarksNoFill() {
        for detail in [proposal(swap: swap(.failed), status: .failed), proposal(swap: nil, status: .executionBlocked)] {
            let spec = build(detail)

            XCTAssertEqual(spec.figure.main, "Passed 4\u{2013}1")
            XCTAssertEqual(spec.figure.context, "the trade didn\u{2019}t go through")
            XCTAssertFalse(spec.chart.markers.contains { $0.kind == .buy })
            assertNoZeroPrice(spec)
        }
    }

    func testTheNotFilledStatsReadAmountPassedAtAndNow() {
        var detail = proposal(swap: nil)
        detail.passedPriceMicros = 709_800_000

        XCTAssertEqual(
            build(detail).stats,
            [
                .init(label: "Amount", value: "$300"), .init(label: "Passed at", value: "$709.80"),
                .init(label: "Now", value: "$724.40"),
            ])
        detail.passedPriceMicros = nil
        detail.stats.priceMicros = nil
        XCTAssertEqual(build(detail).stats.map(\.value), ["$300", "\u{2014}", "\u{2014}"])
    }

    func testAConfirmedSwapWithoutALivePriceKeepsTheFillAndDropsSince() {
        var detail = Components.Schemas.ProposalDetail.meta(now: now)
        detail.stats.priceMicros = nil
        detail.stats.sinceBps = nil
        let spec = build(detail, eyes: ShareEyes(pnlDollars: true))

        XCTAssertEqual(spec.chart.markers.map(\.kind), [.buy])
        XCTAssertEqual(spec.figure.main, "$716.52")
        XCTAssertEqual(spec.figure.context, "filled at")
        XCTAssertNil(spec.figure.secondary)
        XCTAssertEqual(spec.stats.last, .init(label: "Now", value: "\u{2014}"))
    }

    func testAConfirmedSwapWithoutAFillTimeDrawsNoFillMarker() {
        var detail = Components.Schemas.ProposalDetail.meta(now: now)
        detail.swap?.filledAt = nil

        XCTAssertFalse(build(detail).chart.markers.contains { $0.kind == .buy })
    }

    private func filledSell(nowMicros: Int64, sinceBps: Int64) -> Components.Schemas.ProposalDetail {
        let filled = swap(.confirmed, price: 716_520_000, at: now.addingTimeInterval(-17_940))
        var detail = proposal(swap: filled, kind: .sell)
        detail.stats.priceMicros = nowMicros
        detail.stats.sinceBps = sinceBps
        return detail
    }

    func testASellThatFellAfterTheFillIsAvoidedAndGreen() {
        let spec = build(filledSell(nowMicros: 680_694_000, sinceBps: -500))

        XCTAssertEqual(spec.chart.markers.last?.kind, .sell)
        XCTAssertTrue(spec.header.subtitle.contains("Sell META"))
        XCTAssertEqual(spec.figure.main, "Avoided \u{2212}5.0%")
        XCTAssertEqual(spec.figure.context, "since the sale")
        XCTAssertEqual(spec.figure.tone, .gain)
        XCTAssertEqual(
            spec.stats,
            [
                .init(label: "Sold", value: "0.42 sh"), .init(label: "At", value: "$716.52"),
                .init(label: "Now", value: "$680.69"),
            ])
        XCTAssertFalse(spec.spoken.contains("Bought"))
        XCTAssertTrue(spec.spoken.contains("Avoided"), spec.spoken)
    }

    func testASellThatRoseAfterTheFillIsMissedAndRed() {
        let spec = build(filledSell(nowMicros: 752_346_000, sinceBps: 500))

        XCTAssertEqual(spec.figure.main, "Missed +5.0%")
        XCTAssertEqual(spec.figure.context, "since the sale")
        XCTAssertEqual(spec.figure.tone, .loss)
        XCTAssertEqual(spec.stats.first, .init(label: "Sold", value: "0.42 sh"))
    }

    func testAFlatSellIsNeutralWithoutAWord() {
        let spec = build(filledSell(nowMicros: 716_520_000, sinceBps: 0))

        XCTAssertEqual(spec.figure.main, "0.0%")
        XCTAssertEqual(spec.figure.tone, .neutral)
    }

    func testASellWithDollarsOnAvoidsTheDollarChange() {
        let spec = build(filledSell(nowMicros: 680_694_000, sinceBps: -500), eyes: ShareEyes(pnlDollars: true))

        XCTAssertEqual(spec.figure.main, "Avoided \u{2212}$15.05")
        XCTAssertEqual(spec.figure.secondary, "\u{2212}5.0%")
        XCTAssertEqual(spec.figure.tone, .gain)
    }

    func testASellWithoutALivePriceSaysSoldAt() {
        var detail = filledSell(nowMicros: 680_694_000, sinceBps: -500)
        detail.stats.priceMicros = nil
        let spec = build(detail)

        XCTAssertEqual(spec.figure.main, "$716.52")
        XCTAssertEqual(spec.figure.context, "sold at")
    }

    func testABuyKeepsBoughtAndItsSign() {
        var detail = Components.Schemas.ProposalDetail.meta(now: now)
        detail.stats.priceMicros = 680_694_000
        detail.stats.sinceBps = -500
        let spec = build(detail)

        XCTAssertEqual(spec.stats.first?.label, "Bought")
        XCTAssertEqual(spec.figure.main, "\u{2212}5.0%")
        XCTAssertEqual(spec.figure.tone, .loss)
    }

    func testTheBuySellEyeHidesBothMarkersAndCallouts() {
        var detail = Components.Schemas.ProposalDetail.meta(now: now)
        detail.passedPriceMicros = 709_800_000
        let spec = build(detail, eyes: ShareEyes(pnlDollars: false, buySell: false))

        XCTAssertTrue(spec.chart.markers.isEmpty)
        XCTAssertTrue(spec.chart.callouts.isEmpty)
    }

    func testTheHeaderDateIsTheFillsOrThePassesOrTheDeadline() {
        let filled = Components.Schemas.ProposalDetail.meta(now: now)
        XCTAssertEqual(build(filled).header.dateText, day(filled.swap?.filledAt))
        let pending = proposal(swap: nil)
        XCTAssertEqual(build(pending).header.dateText, day(pending.stats.closedAt))
    }

    func testTheCodeEyeHidesTheCode() {
        XCTAssertNil(build(eyes: ShareEyes(pnlDollars: false, code: false)).code)
    }

    private func day(_ date: Date?) -> String {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = utc
        formatter.dateFormat = "MMM d"
        return formatter.string(from: date ?? .distantPast)
    }

    private func assertNoZeroPrice(_ spec: ShareCardSpec, file: StaticString = #filePath, line: UInt = #line) {
        let text =
            [spec.figure.main, spec.figure.context ?? ""] + spec.stats.map(\.value)
            + spec.chart.markers.compactMap(\.callout)
        for value in text {
            XCTAssertFalse(value.contains("$0.00"), value, file: file, line: line)
        }
        XCTAssertTrue(spec.chart.markers.allSatisfy { $0.priceMicros > 0 }, file: file, line: line)
    }
}
