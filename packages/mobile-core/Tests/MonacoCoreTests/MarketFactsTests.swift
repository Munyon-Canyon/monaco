import MonacoAPI
import XCTest

@testable import MonacoCore

final class MarketFactsTests: XCTestCase {
    func testAVoteOpenInOneCabalNamesIt() {
        XCTAssertEqual(
            MarketFacts.subline(.voteOpen(cabalName: "AC Capital", count: 1), ticker: "NVDA"),
            "Vote open in AC Capital")
    }

    func testSeveralOpenVotesAreCounted() {
        XCTAssertEqual(
            MarketFacts.subline(.voteOpen(cabalName: "AC Capital", count: 3), ticker: "NVDA"), "3 votes open")
    }

    func testHeldUnitsDropTrailingZeros() {
        XCTAssertEqual(MarketFacts.subline(.held(units: "24.0000"), ticker: "AAPL"), "24 sh held")
        XCTAssertEqual(MarketFacts.subline(.held(units: "2.2000"), ticker: "TSLA"), "2.2 sh held")
        XCTAssertEqual(MarketFacts.subline(.held(units: "10.0500"), ticker: "AMZN"), "10.05 sh held")
        XCTAssertEqual(MarketFacts.subline(.held(units: "100"), ticker: "AMZN"), "100 sh held")
    }

    func testCalledItGoesThroughResultCopy() {
        XCTAssertEqual(MarketFacts.subline(.called(proposerName: "Christelle"), ticker: "META"), "Christelle called it")
    }

    func testNoFactFallsBackToTheTicker() {
        XCTAssertEqual(MarketFacts.subline(nil, ticker: "MSFT"), "MSFT")
    }

    func testTheSampleMapsEveryKindAndLeavesStocksWithNoFactOut() {
        let facts = MarketFacts.map(.sampleAndres)

        XCTAssertEqual(facts["NVDAx"], .voteOpen(cabalName: "AC Capital", count: 1))
        XCTAssertEqual(facts["AAPLx"], .held(units: "24.0000"))
        XCTAssertEqual(facts["METAx"], .called(proposerName: "Christelle"))
        XCTAssertNil(facts["MSFTx"])
        XCTAssertNil(facts["tSpaceX"])
        XCTAssertEqual(facts.count, 5)
    }
}
