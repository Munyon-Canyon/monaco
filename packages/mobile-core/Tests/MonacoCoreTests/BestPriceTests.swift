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

    private typealias Quote = (id: String, issuer: String, network: String, shares: String, micros: String)

    private func comparison(winner: String, quotes: [Quote]) throws -> Components.Schemas.PriceComparison {
        let now = ISO8601DateFormatter().string(from: Date())
        let rows = quotes.map { quote in
            """
            {"listing_id": "\(quote.id)", "issuer": "\(quote.issuer)", "network": "\(quote.network)",
             "venue": "\(quote.network == "solana" ? "jupiter" : "uniswap")",
             "net_share_nanos": "\(quote.shares)", "net_micros": "\(quote.micros)", "costs": [],
             "quoted_at": "\(now)", "expires_at": "\(now)"}
            """
        }
        let quotes = rows.joined(separator: ",")
        return try decode(#"{"winner_listing_id": "\#(winner)", "quotes": [\#(quotes)], "unavailable": []}"#)
    }

    func testTheOpenAPIExampleIsOneVenueWithItsPriceAndNoAdvantage() throws {
        let example = try decode(
            """
            {"winner_listing_id": "01928a6e-0000-7000-8000-000000000001", "quotes": [{"listing_id":
            "01928a6e-0000-7000-8000-000000000001", "issuer": "xstocks", "network": "solana", "venue": "jupiter",
            "net_share_nanos": "40000000", "net_micros": "10000000", "costs": [], "quoted_at": "2026-10-10T12:00:00Z",
            "expires_at": "2026-10-10T12:00:30Z"}], "unavailable": []}
            """)

        let best = try XCTUnwrap(BestPrice(example, isSell: false))

        XCTAssertEqual(best.headline, "Best price across 1 venue")
        XCTAssertEqual(best.price, "$250.00 a share")
        XCTAssertEqual(best.winner.name, "Solana")
        XCTAssertEqual(best.figure(best.winner), "0.0400 shares")
        XCTAssertNil(best.advantage)
    }

    func testABuyListsEveryVenueWinnerFirstAndCountsTheExtraShares() throws {
        let best = try XCTUnwrap(BestPrice(BestPrice.sampleComparison(now: Date()), isSell: false))

        XCTAssertEqual(best.headline, "Best price across 3 venues")
        XCTAssertEqual(best.venues.map(\.name), ["Base", "Solana", "Ethereum"])
        XCTAssertEqual(best.venues.map(\.isWinner), [true, false, false])
        XCTAssertEqual(best.venues.map(\.network), [.base, .solana, .ethereum])
        XCTAssertEqual(best.venues.map(best.figure), ["0.0401 shares", "0.0400 shares", "0.0398 shares"])
        XCTAssertEqual(best.advantage, "You get 0.0001 more shares than the next best")
    }

    func testTheWinnerLeadsEvenWhenTheServerListsItLater() throws {
        let compared = try comparison(
            winner: "b",
            quotes: [
                ("a", "xstocks", "solana", "40000000", "10000000"), ("b", "ondo", "base", "41000000", "10000000"),
            ])

        let best = try XCTUnwrap(BestPrice(compared, isSell: false))

        XCTAssertEqual(best.winner.id, "b")
        XCTAssertEqual(best.advantage, "You get 0.0010 more shares than the next best")
    }

    func testASellComparesDollarsReceived() throws {
        let compared = try comparison(
            winner: "a",
            quotes: [
                ("a", "xstocks", "solana", "40000000", "10050000"), ("b", "ondo", "base", "40000000", "10000000"),
            ])

        let best = try XCTUnwrap(BestPrice(compared, isSell: true))

        XCTAssertEqual(best.venues.map(best.figure), ["$10.05", "$10.00"])
        XCTAssertEqual(best.advantage, "You get $0.05 more than the next best")
    }

    func testATieOrASubTenThousandthLeadClaimsNoAdvantage() throws {
        let compared = try comparison(
            winner: "a",
            quotes: [
                ("a", "xstocks", "solana", "40050000", "10000000"), ("b", "ondo", "base", "40000000", "10000000"),
            ])

        XCTAssertNil(BestPrice(compared, isSell: false)?.advantage)
    }

    func testUnavailableVenuesFollowWithTheirReasonAndNeverWin() throws {
        let now = ISO8601DateFormatter().string(from: Date())
        let compared = try decode(
            """
            {"winner_listing_id": "a", "quotes": [{"listing_id": "a", "issuer": "xstocks", "network": "solana",
             "venue": "jupiter", "net_share_nanos": "40000000", "net_micros": "10000000", "costs": [],
             "quoted_at": "\(now)", "expires_at": "\(now)"}],
             "unavailable": [
              {"listing_id": "b", "network": "base", "venue": "uniswap", "reason": "simulation_failed"},
              {"listing_id": "c", "network": "ethereum", "venue": "uniswap", "reason": "no_route"},
              {"listing_id": "d", "network": "arbitrum", "venue": "uniswap", "reason": "stale"}]}
            """)

        let best = try XCTUnwrap(BestPrice(compared, isSell: false))

        XCTAssertEqual(best.winner.id, "a")
        XCTAssertEqual(best.headline, "Best price across 1 venue")
        XCTAssertNil(best.advantage)
        XCTAssertEqual(best.unavailable.map(\.id), ["b", "c", "d"])
        XCTAssertEqual(best.unavailable.map(\.name), ["Base", "Ethereum", "Arbitrum"])
        XCTAssertEqual(best.unavailable.map(\.network), [.base, .ethereum, .arbitrum])
        XCTAssertEqual(best.unavailable.map(\.reason), ["Unavailable right now", "No route", "Unavailable right now"])
    }

    func testEachChainIsOneRowWithItsBestQuote() throws {
        let buy = try comparison(
            winner: "base",
            quotes: [
                ("sol-x", "xstocks", "solana", "40000000", "10000000"),
                ("base", "xstocks", "base", "41000000", "10000000"),
                ("sol-o", "ondo", "solana", "40500000", "10000000"),
            ])

        let best = try XCTUnwrap(BestPrice(buy, isSell: false))

        XCTAssertEqual(best.headline, "Best price across 2 venues")
        XCTAssertEqual(best.venues.map(\.name), ["Base", "Solana"])
        XCTAssertEqual(best.venues.map(\.id), ["base", "sol-o"])
        XCTAssertEqual(best.venues.map(\.isWinner), [true, false])
        XCTAssertEqual(best.advantage, "You get 0.0005 more shares than the next best")

        let sell = try comparison(
            winner: "base",
            quotes: [
                ("sol-x", "xstocks", "solana", "40000000", "9900000"),
                ("base", "xstocks", "base", "40000000", "10000000"),
                ("sol-o", "ondo", "solana", "41000000", "9800000"),
            ])

        XCTAssertEqual(try XCTUnwrap(BestPrice(sell, isSell: true)).venues.map(\.id), ["base", "sol-x"])
    }

    func testAChainWithAQuoteShowsNoUnavailableRowAndEachOtherChainShowsOne() throws {
        let now = ISO8601DateFormatter().string(from: Date())
        let compared = try decode(
            """
            {"winner_listing_id": "a", "quotes": [{"listing_id": "a", "issuer": "xstocks", "network": "solana",
             "venue": "jupiter", "net_share_nanos": "40000000", "net_micros": "10000000", "costs": [],
             "quoted_at": "\(now)", "expires_at": "\(now)"}],
             "unavailable": [
              {"listing_id": "b", "network": "solana", "venue": "jupiter", "reason": "no_route"},
              {"listing_id": "c", "network": "base", "venue": "uniswap", "reason": "no_route"},
              {"listing_id": "d", "network": "base", "venue": "uniswap", "reason": "stale"}]}
            """)

        let best = try XCTUnwrap(BestPrice(compared, isSell: false))

        XCTAssertEqual(best.venues.map(\.id), ["a"])
        XCTAssertEqual(best.unavailable.map(\.id), ["c"])
        XCTAssertEqual(best.unavailable.map(\.name), ["Base"])
    }

    func testEveryChainIsLabelledByItsNameAlone() throws {
        let labels = try Components.Schemas.Network.allCases.map { network -> String in
            let quote: Quote = ("a", "xstocks", network.rawValue, "40000000", "10000000")
            let compared = try comparison(winner: "a", quotes: [quote])
            return try XCTUnwrap(BestPrice(compared, isSell: false)).winner.name
        }

        XCTAssertEqual(labels, ["Solana", "Base", "Arbitrum", "Ethereum", "Robinhood Chain"])
    }

    func testEveryUnavailableReasonHasText() {
        let reasons: [Components.Schemas.UnavailableVenue.ReasonPayload] = [
            .noRoute, .stale, .expired, .simulationFailed, .impact,
        ]
        let unavailable = "Unavailable right now"
        XCTAssertEqual(
            reasons.map(BestPrice.reasonText), ["No route", unavailable, unavailable, unavailable, unavailable])
    }

    func testNoComparisonOrAMissingWinnerShowsNothing() throws {
        XCTAssertNil(BestPrice(nil, isSell: false))
        let orphaned = try comparison(winner: "z", quotes: [("a", "xstocks", "solana", "40000000", "10000000")])
        XCTAssertNil(BestPrice(orphaned, isSell: false))
    }
}
