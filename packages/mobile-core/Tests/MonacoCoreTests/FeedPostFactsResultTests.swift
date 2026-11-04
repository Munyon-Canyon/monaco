import MonacoAPI
import MonacoCore
import XCTest

final class FeedPostFactsResultTests: XCTestCase {
    private typealias Item = Components.Schemas.FeedItem
    private typealias Chart = Item.ChartPayload

    private let now =
        Calendar.current.date(from: DateComponents(year: 2026, month: 10, day: 28, hour: 12))
        ?? Date(timeIntervalSinceReferenceDate: 783_000_000)

    func testAPassedBuyIsLedByTheProposerWithTheCabalAsABadge() throws {
        let facts = FeedPostFacts(result(0), now: now)
        guard case .person(let name, _, _, let badge) = facts.lead else { return XCTFail("the proposer leads") }
        XCTAssertEqual(name, "Christelle")
        XCTAssertEqual(badge?.name, "Degens")
        XCTAssertEqual(facts.name, "Christelle\u{2019}s")
        XCTAssertEqual(facts.verb, "META buy passed, 4 to 1")
        XCTAssertEqual(facts.age, "5h")
        XCTAssertEqual(facts.object?.symbol, "METAx")
        XCTAssertEqual(facts.object?.headline, "Buy META $300")
        XCTAssertEqual(facts.object?.place, "Degens")
        XCTAssertEqual(facts.object?.placeIsWarning, false)
        XCTAssertEqual(facts.object?.trailing, .figure(109))
        guard case .result(let seal, let sparkline, let isLoss) = facts.attachment else {
            return XCTFail("a passed listed buy carries the seal")
        }
        XCTAssertEqual(seal, "Christelle")
        XCTAssertEqual(sparkline.count, 32)
        XCTAssertFalse(isLoss)
        XCTAssertEqual(
            facts.spoken,
            "Christelle\u{2019}s META buy passed, 4 to 1, 5 hours ago. "
                + "Buy META 300 dollars, Degens, up 1.1 percent since the fill. Christelle called it.")
    }

    func testAProposerlessPassedBuyKeepsTheCabalLeadAndHeader() {
        var item = result(0)
        item.actorId = nil
        item.actorName = nil
        let facts = FeedPostFacts(item, now: now)
        guard case .cabal(let name, _, _) = facts.lead else { return XCTFail("the cabal leads") }
        XCTAssertEqual(name, "Degens")
        XCTAssertEqual(facts.name, "Degens")
        XCTAssertEqual(facts.verb, "passed it 4–1")
        XCTAssertEqual(facts.object?.place, "Degens")
        XCTAssertFalse(facts.spoken.contains("filled"))
        item.fillState = .pending
        XCTAssertEqual(FeedPostFacts(item, now: now).object?.place, "Buying now")
    }

    func testASaleNamesThePersonAndCarriesItsChartWithBothMarkers() throws {
        var item = result(1)
        let buy = now.addingTimeInterval(-3 * 86_400)
        item.chart = chart(markers: [(buy, .buy), (now, .sell)])
        let facts = FeedPostFacts(item, now: now)
        guard case .person(let name, _, _, _) = facts.lead else { return XCTFail("the proposer leads") }
        XCTAssertEqual(name, "Logan")
        XCTAssertEqual(facts.verb, "sold")
        XCTAssertEqual(facts.object?.headline, "AAPL $480")
        XCTAssertEqual(facts.object?.place, "Tech Titans")
        XCTAssertEqual(facts.object?.trailing, .figure(670))
        guard case .sale(let points, let markers, let isLoss) = facts.attachment else {
            return XCTFail("a filled sale carries its chart")
        }
        XCTAssertEqual(points.count, 3)
        XCTAssertEqual(markers.map(\.side), [.buy, .sell])
        XCTAssertEqual(markers.map(\.at), [buy, now])
        XCTAssertFalse(isLoss)
        let day = Calendar.current.component(.day, from: buy)
        let month = Calendar.current.monthSymbols[Calendar.current.component(.month, from: buy) - 1]
        XCTAssertEqual(
            facts.spoken,
            "Logan sold AAPL, 480 dollars, in Tech Titans, 5 hours ago. Up 6.7 percent on what the cabal paid. "
                + "Bought \(month) \(day), sold today.")
    }

    func testASaleAtALossIsRedAndSpokenAsDown() {
        var item = result(1)
        item.actorName = "Priya"
        item.symbol = "AMZNx"
        item.usdcMicros = 250_000_000
        item.cabalName = "Degens"
        item.returnBps = -210
        let facts = FeedPostFacts(item, now: now)
        XCTAssertEqual(facts.object?.headline, "AMZN $250")
        XCTAssertEqual(facts.object?.trailing, .figure(-210))
        guard case .sale(_, _, let isLoss) = facts.attachment else { return XCTFail("a sale") }
        XCTAssertTrue(isLoss)
        XCTAssertTrue(facts.spoken.contains("Down 2.1 percent on what the cabal paid."))
    }

    func testAPreIPOBuyNeverShowsAPercentOrAChart() {
        var item = result(2)
        item.returnBps = 500
        item.chart = chart(markers: [])
        let facts = FeedPostFacts(item, now: now)
        guard case .person(let name, _, _, _) = facts.lead else { return XCTFail("the proposer leads") }
        XCTAssertEqual(name, "Sam")
        XCTAssertEqual(facts.verb, "bought")
        XCTAssertEqual(facts.age, "4d")
        XCTAssertEqual(facts.object?.headline, "OpenAI $200")
        XCTAssertEqual(facts.object?.place, "Quant Kids")
        XCTAssertEqual(facts.object?.trailing, .text("Pre-IPO"))
        XCTAssertEqual(facts.attachment, .none)
        XCTAssertFalse(facts.spoken.contains("percent"))
    }

    func testAFailedVoteNamesTheProposerAndTradesNothing() {
        let facts = FeedPostFacts(result(4), now: now)
        guard case .person(let name, _, _, let badge) = facts.lead else { return XCTFail("the proposer leads") }
        XCTAssertEqual(name, "Logan")
        XCTAssertEqual(badge?.name, "Tech Titans")
        XCTAssertEqual(facts.name, "Logan\u{2019}s")
        XCTAssertEqual(facts.verb, "TSLA sale failed, 2 to 2")
        XCTAssertEqual(facts.object?.headline, "Sell TSLA $900")
        XCTAssertEqual(facts.object?.place, "Tech Titans")
        XCTAssertNil(facts.object?.trailing)
        XCTAssertEqual(facts.attachment, .none)
        XCTAssertEqual(
            facts.spoken,
            "Logan\u{2019}s TSLA sale failed, 2 to 2, 4 days ago. Sell TSLA 900 dollars, Tech Titans.")
    }

    func testAPassWaitingOnItsSwapsReadsBuyingNowEvenWithOneOfTwoConfirmed() {
        var item = result(0)
        item.fillState = .pending
        item.filledUsdcMicros = 150_000_000
        let facts = FeedPostFacts(item, now: now)
        XCTAssertEqual(facts.verb, "META buy passed, 4 to 1")
        XCTAssertEqual(facts.object?.headline, "Buy META $300")
        XCTAssertEqual(facts.object?.place, "Buying now")
        XCTAssertNil(facts.object?.trailing)
        XCTAssertEqual(facts.attachment, .none)
        XCTAssertTrue(facts.spoken.contains("Buy META 300 dollars, Buying now."))
        item.action = "sell"
        XCTAssertEqual(FeedPostFacts(item, now: now).object?.place, "Selling now")
    }

    func testAPartialFillSaysSoInTheWarningColour() {
        let facts = FeedPostFacts(result(3), now: now)
        XCTAssertEqual(facts.verb, "NVDA buy passed, 4 to 1")
        XCTAssertEqual(facts.object?.place, "Half filled")
        XCTAssertEqual(facts.object?.placeIsWarning, true)
        XCTAssertNil(facts.object?.trailing)
        XCTAssertEqual(facts.attachment, .none)
    }

    func testAFailedTradeSaysSoInTheWarningColour() {
        var item = result(0)
        item.fillState = .failed
        let facts = FeedPostFacts(item, now: now)
        XCTAssertEqual(facts.object?.place, "Didn\u{2019}t go through")
        XCTAssertEqual(facts.object?.placeIsWarning, true)
        XCTAssertEqual(facts.attachment, .none)
        for old in ["filled", "no trade", "trade failed", "partly"] {
            XCTAssertFalse(facts.spoken.lowercased().contains(old), old)
        }
    }

    func testTheLeadFollowsTheShapeNotTheOrder() {
        var item = result(0)
        item.shape = .member
        let facts = FeedPostFacts(item, now: now)
        guard case .person(let name, _, _, _) = facts.lead else { return XCTFail("the proposer leads") }
        XCTAssertEqual(name, "Christelle")
        XCTAssertEqual(facts.verb, "passed it 4–1")
        XCTAssertEqual(facts.object?.place, "Degens")
        XCTAssertTrue(
            facts.spoken.hasPrefix("Christelle passed Buy META, 300 dollars, 4 to 1, in Degens, 5 hours ago."))
    }

    func testANullReturnShowsNoFigureAndAZeroShowsZero() {
        var item = result(0)
        item.returnBps = nil
        item.chart = nil
        var facts = FeedPostFacts(item, now: now)
        XCTAssertNil(facts.object?.trailing)
        XCTAssertEqual(facts.attachment, .result(sealName: "Christelle", sparkline: [], isLoss: false))
        XCTAssertTrue(facts.spoken.hasSuffix("Degens. Christelle called it."))
        item.returnBps = 0
        facts = FeedPostFacts(item, now: now)
        XCTAssertEqual(facts.object?.trailing, .figure(0))
        XCTAssertEqual(PercentFormatter.format(basisPoints: 0, signed: true, fractionDigits: 1), "0.0%")
        XCTAssertTrue(facts.spoken.contains("unchanged since the fill."))
    }

    func testAPassWithNoCallerHasNoSeal() {
        var item = result(0)
        item.caller = nil
        XCTAssertEqual(FeedPostFacts(item, now: now).attachment, .none)
    }

    func testASaleWithNoChartKeepsItsFigureAndDrawsNothing() {
        var item = result(1)
        item.chart = nil
        let facts = FeedPostFacts(item, now: now)
        XCTAssertEqual(facts.object?.trailing, .figure(670))
        XCTAssertEqual(facts.attachment, .none)
    }

    func testPercentsPrintWithASignAndATypographicMinus() {
        XCTAssertEqual(PercentFormatter.format(basisPoints: 109, signed: true, fractionDigits: 1), "+1.1%")
        XCTAssertEqual(PercentFormatter.format(basisPoints: -210, signed: true, fractionDigits: 1), "\u{2212}2.1%")
    }

    func testTheTallyUsesAnEnDash() {
        var item = result(0)
        item.actorId = nil
        XCTAssertTrue(FeedPostFacts(item, now: now).verb.contains("\u{2013}"))
        item = result(4)
        item.actorId = nil
        XCTAssertEqual(FeedPostFacts(item, now: now).verb, "failed it 2–2")
    }

    private func result(_ index: Int) -> Item {
        var item = Item.results[index]
        let ages: [TimeInterval] = [18_000, 18_000, 345_600, 172_800, 345_600]
        let age = ages[index]
        item.createdAt = now.addingTimeInterval(-age)
        item.updatedAt = item.createdAt
        return item
    }

    private func chart(markers: [(Date, Chart.MarkersPayloadPayload.SidePayload)]) -> Chart {
        let points: [Chart.PointsPayloadPayload] = (0..<3).map { step in
            let offset: TimeInterval = TimeInterval(step - 3) * 3600
            let price: Int64 = 100_000_000 + Int64(step)
            return Chart.PointsPayloadPayload(at: now.addingTimeInterval(offset), priceMicros: price)
        }
        let marks: [Chart.MarkersPayloadPayload] = markers.map { marker in
            Chart.MarkersPayloadPayload(at: marker.0, side: marker.1, priceMicros: 100_000_000)
        }
        return Chart(points: points, markers: marks)
    }
}
