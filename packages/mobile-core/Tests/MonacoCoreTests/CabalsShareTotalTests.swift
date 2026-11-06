import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalsShareTotalTests: XCTestCase {
    func testTheShareTotalIsTheSumOfTheTilesAndMatchesHomesFigure() async throws {
        let store = try makeStore(.sampleOwnProfile)

        await store.tab.load()

        let summary = PortfolioSummary(.sampleOwnProfile)
        XCTAssertEqual(store.shareTotalText, "$11,582.00")
        XCTAssertEqual(store.shareTotalText, HomeFigureCopy.partFigure(micros: summary.totalMicros))
        XCTAssertEqual(store.shareTotalSpoken, "11,582 dollars")
        XCTAssertEqual(try XCTUnwrap(store.content).shareTotalMicros, summary.totalMicros)
    }

    func testTheShareTotalLeavesOutInvitesAndFloorsToTheCent() async throws {
        var portfolio = Components.Schemas.MyPortfolio.sampleOwnProfile
        portfolio.cabals[2].valueMicros += 459_999
        let store = try makeStore(portfolio)

        await store.tab.load()

        XCTAssertEqual(try XCTUnwrap(store.content).invites.count, 1)
        XCTAssertEqual(store.shareTotalText, "$11,582.45")
        XCTAssertEqual(store.shareTotalSpoken, "11,582 dollars 45 cents")
    }

    func testTheShareTotalIsAbsentWhenNoTileIsFunded() async throws {
        var portfolio = Components.Schemas.MyPortfolio.sampleLoss
        for index in portfolio.cabals.indices { portfolio.cabals[index].valueMicros = 0 }
        let store = try makeStore(portfolio, invites: "[]")
        XCTAssertNil(store.shareTotalText)
        XCTAssertNil(store.shareTotalSpoken)

        await store.tab.load()

        XCTAssertEqual(store.tiles.count, 2)
        XCTAssertNil(store.shareTotalText)
        XCTAssertNil(store.shareTotalSpoken)
    }

    func testTheSpokenDollarsSayOneDollarInTheSingular() {
        XCTAssertEqual(CabalsTabCopy.spokenDollars(figure: "$1.00"), "1 dollar")
        XCTAssertEqual(CabalsTabCopy.spokenDollars(figure: "$1.01"), "1 dollar 1 cent")
        XCTAssertEqual(CabalsTabCopy.spokenDollars(figure: "$0.50"), "0 dollars 50 cents")
    }

    private func makeStore(
        _ portfolio: Components.Schemas.MyPortfolio, invites: String? = nil
    ) throws -> CabalsTabStore {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let body = String(decoding: try encoder.encode(portfolio), as: UTF8.self)
        let transport = StubTransport(routes: [
            "/v1/me/cabals": [.json(.ok, "[]")],
            "/v1/me/portfolio": [.json(.ok, body)],
            "/v1/me/cabal-invites": [.json(.ok, invites ?? Self.invite)],
        ])
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return CabalsTabStore(api: api, hints: FakeHintStream(), portfolio: nil)
    }

    private static let invite = """
        [{"request_id":"01890a5d-ac96-774b-bcce-b302099a8059","cabal":{"id":"01890a5d-ac96-774b-bcce-b302099a805d","name":"Quant Kids","picture_url":null,"member_count":5},"invited_by":{"user_id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"kai","display_name":"Kai","photo_url":null},"expires_at":"2026-10-09T15:00:00Z"}]
        """
}
