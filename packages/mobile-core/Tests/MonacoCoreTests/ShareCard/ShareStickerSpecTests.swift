import Foundation
import XCTest

@testable import MonacoCore

final class ShareStickerSpecTests: XCTestCase {
    private let utc = TimeZone(identifier: "UTC") ?? .gmt

    private static func date(_ month: Int, _ day: Int) -> Date {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        return calendar.date(from: DateComponents(year: 2026, month: month, day: day, hour: 15)) ?? .distantPast
    }

    private var crest: ShareCrest { ShareCrest(cabalID: "cabal-1", name: "AC Capital", pictureURL: nil) }

    private func card(facts: ShareFacts, eyes: ShareEyes = ShareEyes()) -> ShareCardSpec {
        ShareCardSpec(
            header: .init(mark: .crest(crest), title: "AC Capital", subtitle: "", dateText: ""),
            chart: ShareChart(points: [], markers: []),
            tickerLine: .init(logoURL: nil, symbol: "NVDA", detail: "Long"),
            figure: eyes.pnlDollars
                ? .init(main: "+$1,027.90", secondary: "+14.0%", context: nil, tone: .gain)
                : .init(main: "+14.0%", secondary: nil, context: nil, tone: .gain),
            stats: [], code: eyes.code ? "ANDRES" : nil, replayTitle: "", facts: facts, spoken: "spoken")
    }

    private var position: ShareFacts { ShareFacts(symbol: "NVDA", firstBoughtAt: Self.date(6, 3), side: .long) }

    private func sticker(_ facts: ShareFacts, _ eyes: ShareEyes) -> ShareStickerSpec {
        ShareStickerSpec(from: card(facts: facts, eyes: eyes), eyes: eyes, timeZone: utc)
    }

    func testTheOffFrameIsTheCrestSymbolAndPercentAlone() {
        let off = ShareEyes(pnlDollars: false, buySell: false, code: false)
        let spec = sticker(position, off)

        XCTAssertEqual(spec.mark, .crest(crest))
        XCTAssertEqual(spec.name, "AC Capital")
        XCTAssertEqual(spec.ticker, .init(logoURL: nil, symbol: "NVDA", detail: nil))
        XCTAssertEqual(spec.figureMain, "+14.0%")
        XCTAssertNil(spec.figureSecondary)
        XCTAssertNil(spec.code)
        XCTAssertEqual(spec.spoken, "spoken")
    }

    func testTheOnFrameAddsEachEyesField() {
        let spec = sticker(position, ShareEyes())

        XCTAssertEqual(spec.ticker?.detail, "long since Jun 3")
        XCTAssertEqual(spec.figureMain, "+$1,027.90")
        XCTAssertEqual(spec.figureSecondary, "+14.0%")
        XCTAssertEqual(spec.code, "ANDRES")
        XCTAssertEqual(spec.tone, .gain)
    }

    func testEachEyeAddsOnlyItsField() {
        let buySell = sticker(position, ShareEyes(pnlDollars: false, buySell: true, code: false))
        XCTAssertEqual(buySell.ticker?.detail, "long since Jun 3")
        XCTAssertEqual(buySell.figureMain, "+14.0%")
        XCTAssertNil(buySell.code)

        let code = sticker(position, ShareEyes(pnlDollars: false, buySell: false, code: true))
        XCTAssertNil(code.ticker?.detail)
        XCTAssertEqual(code.code, "ANDRES")
    }

    func testAShortSaysShort() {
        let facts = ShareFacts(symbol: "NVDA", firstBoughtAt: Self.date(6, 3), side: .short)

        XCTAssertEqual(sticker(facts, ShareEyes()).ticker?.detail, "short since Jun 3")
    }

    func testASubjectWithNoFirstBuyShowsTheSymbolAlone() {
        let facts = ShareFacts(symbol: "NVDA", firstBoughtAt: nil, side: .long)

        XCTAssertEqual(sticker(facts, ShareEyes()).ticker, .init(logoURL: nil, symbol: "NVDA", detail: nil))
    }

    func testASubjectWithNoSymbolHasNoTickerLine() {
        XCTAssertNil(sticker(ShareFacts(), ShareEyes()).ticker)
    }

    func testAPercentOnlyCardStaysPercentWhenDollarsAreOn() {
        let spec = ShareStickerSpec(
            from: card(facts: position, eyes: ShareEyes(pnlDollars: false)), eyes: ShareEyes(), timeZone: utc)

        XCTAssertEqual(spec.figureMain, "+14.0%")
        XCTAssertNil(spec.figureSecondary)
    }
}
