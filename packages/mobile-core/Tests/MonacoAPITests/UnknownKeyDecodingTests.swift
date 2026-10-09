import Foundation
import MonacoAPI
import XCTest

final class UnknownKeyDecodingTests: XCTestCase {
    private func decode<T: Decodable>(_: T.Type, _ raw: String) throws -> T {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(T.self, from: Data(raw.utf8))
    }

    func testAFeedItemWithAnUnknownKeyDecodes() throws {
        let raw =
            ##"{"id":"01920000-0000-7000-8000-000000000007","kind":"trade","ref_type":"swaps","##
            + ##""ref_id":"01920000-0000-7000-8000-000000000008","cabal_id":null,"cabal_name":null,"##
            + ##""actor_id":null,"actor_name":null,"actor_photo_url":null,"asset_id":null,"symbol":"AAPLx","##
            + ##""title":"Alpha Cabal bought $500 of AAPLx","detail":null,"body":null,"status":null,"##
            + ##""tone":"neutral","comment_count":0,"created_at":"2026-10-03T08:00:00Z","##
            + ##""updated_at":"2026-10-03T08:00:00Z","added_next_release":{"nested":[1,2]}}"##

        let item = try decode(Components.Schemas.FeedItem.self, raw)

        XCTAssertEqual(item.symbol, "AAPLx")
        XCTAssertEqual(item.commentCount, 0)
    }

    func testAnAssetSummaryWithAnUnknownKeyDecodes() throws {
        let raw =
            ##"{"symbol":"AAPLx","display_name":"Apple","issuer":"xstocks","kind":"equity","##
            + ##""logo_url":"https://cdn.example.com/AAPLx.png","price_micros":110000000,"##
            + ##""price_as_of":"2026-03-04T14:30:00Z","change_bps":1000,"sparkline_micros":[100000000,110000000],"##
            + ##""session":{"state":"open","continuous":false,"holiday":"","early_close":false,"##
            + ##""next_state":"after_hours","next_transition":"2026-03-04T21:00:00Z","extra_session_key":true},"##
            + ##""tradable":true,"quotable":true}"##

        let asset = try decode(Components.Schemas.AssetSummary.self, raw)

        XCTAssertEqual(asset.symbol, "AAPLx")
        XCTAssertEqual(asset.changeBps, 1000)
    }
}
