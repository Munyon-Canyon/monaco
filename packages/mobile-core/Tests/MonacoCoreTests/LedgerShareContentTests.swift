import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore
@testable import MonacoRanking

final class LedgerShareContentTests: XCTestCase {
    private let utc = TimeZone(identifier: "UTC") ?? .gmt
    private let saturday = Date(timeIntervalSince1970: 1_792_259_200)

    private func cabal(_ n: Int, picture: String? = nil) -> Components.Schemas.LedgerCabal {
        .init(id: "c\(n)", name: "Cabal \(n)", pictureUrl: picture)
    }

    private func ledger(
        cabals: [Components.Schemas.LedgerCabal]? = nil, edit: (inout Components.Schemas.Ledger) -> Void = { _ in }
    ) -> Components.Schemas.Ledger {
        var value = Components.Schemas.Ledger.week42
        if let cabals { value.cabals = cabals }
        edit(&value)
        return value
    }

    private func content(_ ledger: Components.Schemas.Ledger, code: String? = "andres") -> LedgerShareContent {
        LedgerShareContent(ledger, name: "Andres", photoURL: nil, code: code, timeZone: utc)
    }

    func testTheSublineNamesUpToThreeCabalsThenCountsTheRest() {
        let expected = [
            "Oct 12 – 18 · Cabal 1",
            "Oct 12 – 18 · Cabal 1, Cabal 2",
            "Oct 12 – 18 · Cabal 1, Cabal 2, Cabal 3",
            "Oct 12 – 18 · Cabal 1, Cabal 2, Cabal 3 and 1 more",
            "Oct 12 – 18 · Cabal 1, Cabal 2, Cabal 3 and 2 more",
        ]
        for count in 1...5 {
            let value = content(ledger(cabals: (1...count).map { cabal($0) }))
            XCTAssertEqual(value.subline, expected[count - 1])
        }
    }

    func testTheSublineFallsBackWhenTheDatesOrTheCabalsAreMissing() {
        XCTAssertEqual(content(ledger { $0.labelStart = "bad" }).subline, "Degens, AC Capital, Tech Titans")
        XCTAssertEqual(content(ledger(cabals: []) { $0.labelEnd = "bad" }).subline, "")
        XCTAssertEqual(content(ledger(cabals: [])).subline, "Oct 12 – 18")
    }

    func testTheHeaderFigureStatsAndCodeComeFromTheLedger() {
        let value = content(ledger())

        XCTAssertEqual(value.header, "Andres · Week 42")
        XCTAssertEqual(value.weekTitle, "Week 42")
        XCTAssertEqual(value.figure, "+1.4%")
        XCTAssertFalse(value.isLoss)
        XCTAssertEqual(value.bestCall, "+4.2%")
        XCTAssertEqual(value.sharpest, "Christelle")
        XCTAssertEqual(value.voted, "3 of 3")
        XCTAssertEqual(value.code, "ANDRES")
        XCTAssertEqual(value.spokenReturn, "up 1.4 percent")
        XCTAssertEqual(value.spokenBestCall, "up 4.2 percent")
    }

    func testAWeekWithoutANumberReadsThisWeek() {
        XCTAssertEqual(LedgerShareContent.weekTitle("nope"), "This week")
    }

    func testNoBestCallShowsADashAndALossIsALoss() {
        let value = content(
            ledger {
                $0.bestCall = nil
                $0.stakeReturnBps = -80
            })

        XCTAssertEqual(value.bestCall, "—")
        XCTAssertNil(value.spokenBestCall)
        XCTAssertTrue(value.isLoss)
        XCTAssertEqual(value.spokenReturn, "down 0.8 percent")
    }

    func testAFlatOrUnmeasuredReturn() {
        XCTAssertEqual(content(ledger { $0.stakeReturnBps = 0 }).spokenReturn, "flat")
        let none = content(ledger { $0.stakeReturnBps = nil })
        XCTAssertNil(none.spokenReturn)
        XCTAssertEqual(none.figure, "—")
    }

    func testTheCoverIsTheFirstCabalWithAPicture() {
        let cabals = [cabal(1, picture: "https://x.test/1.png"), cabal(2, picture: "https://x.test/2.png")]

        let cover = content(ledger(cabals: cabals)).cover

        XCTAssertEqual(cover.cabalID, "c1")
        XCTAssertEqual(cover.pictureURL?.absoluteString, "https://x.test/1.png")
    }

    func testTheCoverSkipsACabalWithoutAPicture() {
        let cabals = [cabal(1), cabal(2, picture: ""), cabal(3, picture: "https://x.test/3.png")]

        let cover = content(ledger(cabals: cabals)).cover

        XCTAssertEqual(cover.cabalID, "c3")
        XCTAssertEqual(cover.pictureURL?.absoluteString, "https://x.test/3.png")
    }

    func testTheCoverFallsBackToTheFirstCabalsCrestAndThenTheViewer() {
        let crest = content(ledger(cabals: [cabal(1), cabal(2)])).cover
        XCTAssertEqual(crest, .init(cabalID: "c1", name: "Cabal 1", pictureURL: nil))

        let none = content(ledger(cabals: [])).cover
        XCTAssertEqual(none, .init(cabalID: "", name: "Andres", pictureURL: nil))
    }

    func testTheCallMarkerNamesTheWeekdayInTheGivenZone() {
        XCTAssertEqual(
            LedgerShareContent.callLabel(symbol: "AMZN", at: saturday, timeZone: utc), "Called AMZN · Sat")
        let tokyo = TimeZone(identifier: "Asia/Tokyo") ?? .gmt
        XCTAssertEqual(LedgerShareContent.callLabel(symbol: "AMZN", at: saturday, timeZone: tokyo), "Called AMZN · Sun")
    }

    func testMarkersAndPointsCarryOver() {
        let value = content(
            ledger {
                $0.series = [
                    .init(at: saturday.addingTimeInterval(-3_600), returnBps: 10), .init(at: saturday, returnBps: 140),
                ]
                $0.markers = [
                    .init(at: saturday, kind: .call, symbol: "AMZN", label: "x"),
                    .init(at: saturday, kind: .buy, symbol: "NVDA", label: "y"),
                ]
            })

        XCTAssertEqual(value.points.map(\.returnBps), [10, 140])
        XCTAssertEqual(value.markers.map(\.label), ["Called AMZN · Sat", "B"])
    }

    private func spec(eyes: ShareEyes = ShareEyes(), edit: (inout Components.Schemas.Ledger) -> Void = { _ in })
        -> ShareCardSpec
    {
        var l = ledger()
        l.series = [.init(at: saturday.addingTimeInterval(-3_600), returnBps: 10), .init(at: saturday, returnBps: 140)]
        l.markers = [
            .init(at: saturday, kind: .call, symbol: "AMZN", label: "x"),
            .init(at: saturday.addingTimeInterval(-3_000), kind: .buy, symbol: "NVDA", label: "y"),
        ]
        edit(&l)
        return ShareLedgerBuilder.build(content: content(l), eyes: eyes)
    }

    func testTheCardShowsEverythingByDefault() {
        let card = spec()

        XCTAssertTrue(card.facts.isLedger)
        XCTAssertEqual(card.header.title, "Andres · Week 42")
        XCTAssertEqual(card.figure.main, "+1.4%")
        XCTAssertEqual(card.figure.context, "this week")
        XCTAssertEqual(card.stats.map(\.value), ["+4.2%", "Christelle", "3 of 3"])
        XCTAssertEqual(card.code, "ANDRES")
        XCTAssertEqual(card.replayTitle, "Replay the week")
        XCTAssertEqual(card.chart.markers.map(\.kind), [.seal, .buy])
        XCTAssertEqual(card.chart.markers.first?.callout, "Called AMZN · Sat")
        XCTAssertEqual(card.chart.markers.first?.priceMicros, 140)
        XCTAssertEqual(
            card.spoken,
            "Andres, week 42, up 1.4 percent. Best call up 4.2 percent. Sharpest Christelle. Voted 3 of 3. Join with code ANDRES."
        )
    }

    func testReturnOffTurnsTheFigureIntoTheWeek() {
        let card = spec(eyes: ShareEyes(pnlDollars: false))

        XCTAssertEqual(card.figure.main, "Week 42")
        XCTAssertNil(card.figure.context)
        XCTAssertFalse(card.spoken.contains("percent, "))
        XCTAssertTrue(card.spoken.hasPrefix("Andres, week 42. Best call"))
    }

    func testCallsOffDropsTheStatsAndTheMarkerLabels() {
        let card = spec(eyes: ShareEyes(buySell: false))

        XCTAssertTrue(card.stats.isEmpty)
        XCTAssertNil(card.chart.markers.first?.callout)
        XCTAssertEqual(card.chart.markers.count, 2)
        XCTAssertEqual(card.spoken, "Andres, week 42, up 1.4 percent. Join with code ANDRES.")
    }

    func testCodeOffDropsTheCodeLine() {
        let card = spec(eyes: ShareEyes(code: false))

        XCTAssertNil(card.code)
        XCTAssertFalse(card.spoken.contains("Join with code"))
    }

    func testNoSeriesMeansNoMarkersAndNoBestCallIsLeftOutOfTheSpeech() {
        let card = spec {
            $0.series = []
            $0.bestCall = nil
        }

        XCTAssertTrue(card.chart.points.isEmpty)
        XCTAssertTrue(card.chart.markers.isEmpty)
        XCTAssertFalse(card.spoken.contains("Best call"))
    }
}
