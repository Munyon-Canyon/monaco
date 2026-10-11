import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class BestPriceTests: XCTestCase {
    private func decode(_ json: String) throws -> Components.Schemas.PriceComparison {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(Components.Schemas.PriceComparison.self, from: Data(json.utf8))
    }

    private typealias Quote = (id: String, issuer: String, venue: String, shares: String, micros: String)

    private func comparison(winner: String, quotes: [Quote]) throws -> Components.Schemas.PriceComparison {
        let now = ISO8601DateFormatter().string(from: Date())
        let rows = quotes.map { quote in
            """
            {"listing_id": "\(quote.id)", "issuer": "\(quote.issuer)", "network": "solana", "venue": "\(quote.venue)",
             "net_share_nanos": "\(quote.shares)", "net_micros": "\(quote.micros)", "costs": [],
             "quoted_at": "\(now)", "expires_at": "\(now)"}
            """
        }
        return try decode(#"{"winner_listing_id": "\#(winner)", "quotes": [\#(rows.joined(separator: ","))]}"#)
    }

    func testTheOpenAPIExampleIsOneVenueWithItsPriceAndNoAdvantage() throws {
        let example = try decode(
            """
            {"winner_listing_id": "01928a6e-0000-7000-8000-000000000001", "quotes": [{"listing_id":
            "01928a6e-0000-7000-8000-000000000001", "issuer": "xstocks", "network": "solana", "venue": "jupiter",
            "net_share_nanos": "40000000", "net_micros": "10000000", "costs": [], "quoted_at": "2026-10-10T12:00:00Z",
            "expires_at": "2026-10-10T12:00:30Z"}]}
            """)

        let best = try XCTUnwrap(BestPrice(example, isSell: false))

        XCTAssertEqual(best.headline, "Best price across 1 venue")
        XCTAssertEqual(best.price, "$250.00 a share")
        XCTAssertEqual(best.winner.name, "Jupiter on Solana")
        XCTAssertEqual(best.figure(best.winner), "0.0400 shares")
        XCTAssertNil(best.advantage)
    }

    func testABuyListsEveryVenueWinnerFirstAndCountsTheExtraShares() throws {
        let best = try XCTUnwrap(BestPrice(BestPrice.sampleComparison(now: Date()), isSell: false))

        XCTAssertEqual(best.headline, "Best price across 3 venues")
        XCTAssertEqual(best.venues.map(\.name), ["Uniswap on Base", "Jupiter on Solana", "Uniswap on Ethereum"])
        XCTAssertEqual(best.venues.map(\.isWinner), [true, false, false])
        XCTAssertEqual(best.venues.map(best.figure), ["0.0401 shares", "0.0400 shares", "0.0398 shares"])
        XCTAssertEqual(best.advantage, "You get 0.0001 more shares than the next best")
    }

    func testTheWinnerLeadsEvenWhenTheServerListsItLater() throws {
        let compared = try comparison(
            winner: "b",
            quotes: [
                ("a", "xstocks", "jupiter", "40000000", "10000000"), ("b", "ondo", "uniswap", "41000000", "10000000"),
            ])

        let best = try XCTUnwrap(BestPrice(compared, isSell: false))

        XCTAssertEqual(best.winner.id, "b")
        XCTAssertEqual(best.advantage, "You get 0.0010 more shares than the next best")
    }

    func testASellComparesDollarsReceived() throws {
        let compared = try comparison(
            winner: "a",
            quotes: [
                ("a", "xstocks", "jupiter", "40000000", "10050000"), ("b", "ondo", "uniswap", "40000000", "10000000"),
            ])

        let best = try XCTUnwrap(BestPrice(compared, isSell: true))

        XCTAssertEqual(best.venues.map(best.figure), ["$10.05", "$10.00"])
        XCTAssertEqual(best.advantage, "You get $0.05 more than the next best")
    }

    func testATieOrASubTenThousandthLeadClaimsNoAdvantage() throws {
        let compared = try comparison(
            winner: "a",
            quotes: [
                ("a", "xstocks", "jupiter", "40050000", "10000000"), ("b", "ondo", "uniswap", "40000000", "10000000"),
            ])

        XCTAssertNil(BestPrice(compared, isSell: false)?.advantage)
    }

    func testNoComparisonOrAMissingWinnerShowsNothing() throws {
        XCTAssertNil(BestPrice(nil, isSell: false))
        let orphaned = try comparison(winner: "z", quotes: [("a", "xstocks", "jupiter", "40000000", "10000000")])
        XCTAssertNil(BestPrice(orphaned, isSell: false))
    }
}
