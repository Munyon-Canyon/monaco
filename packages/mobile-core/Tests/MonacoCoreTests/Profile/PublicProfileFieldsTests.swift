import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class PublicProfileFieldsTests: XCTestCase {
    private static let userID = "01890a5d-ac96-774b-bcce-b302099a8058"

    func testASharedReturnMapsToAFigure() async {
        var profile = Components.Schemas.PublicProfile.sample
        profile.return1mBps = -348
        profile.pnl1mMicros = -417_680_000
        XCTAssertEqual(PublicPerformance(profile), .figure(pnlMicros: -417_680_000, returnBps: -348))
    }

    func testAHiddenReturnWinsOverAnyFigure() async {
        var profile = Components.Schemas.PublicProfile.sample
        profile.returnPrivate = true
        XCTAssertEqual(PublicPerformance(profile), .hidden)
    }

    func testAMissingReturnIsUnavailableNotPrivate() async {
        var profile = Components.Schemas.PublicProfile.sample
        profile.return1mBps = nil
        profile.pnl1mMicros = nil
        XCTAssertEqual(PublicPerformance(profile), .unavailable)
        profile.return1mBps = 820
        XCTAssertEqual(PublicPerformance(profile), .unavailable)
    }

    func testTheModelServesTheBioJoinedDateCabalCountAndReturn() async throws {
        let profile = Components.Schemas.PublicProfile.sample
        let transport = StubTransport(.json(.ok, try Self.json(profile)))
        let model = UserProfileModel(
            userID: Self.userID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport))

        XCTAssertNil(model.performance)
        await model.load()

        XCTAssertEqual(model.bio, "Poet.\nSan Francisco")
        XCTAssertEqual(model.joinedAt, Date(timeIntervalSince1970: 1_759_233_600))
        XCTAssertEqual(model.cabalCount, 3)
        XCTAssertEqual(model.performance, .figure(pnlMicros: 41_250_000, returnBps: 820))
    }

    func testAPublicActivityReadAsksForTheActorsProposalsAndReplies() async throws {
        let items = Array(Components.Schemas.FeedItem.samples.prefix(2))
        let page = Components.Schemas.FeedPage(items: items, nextCursor: nil)
        let transport = StubTransport(.json(.ok, try Self.json(page)))
        let model = PublicActivityModel(
            userID: Self.userID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport))

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.items.map(\.id), items.map(\.id))
        let sent = await transport.sent
        let query = URLComponents(string: sent.first?.path ?? "")?.queryItems ?? []
        let values = Dictionary(query.map { ($0.name, $0.value ?? "") }, uniquingKeysWith: { _, last in last })
        XCTAssertEqual(values["actor_id"], Self.userID)
        XCTAssertEqual(values["kind"], "proposal,reply")
        XCTAssertEqual(values["limit"], "5")
    }

    func testAFailedPublicActivityReadLeavesNoRows() async throws {
        let transport = StubTransport(.json(.internalServerError, "{}"))
        let model = PublicActivityModel(
            userID: Self.userID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport))

        await model.load()

        XCTAssertEqual(model.phase, .failed)
        XCTAssertTrue(model.items.isEmpty)
    }

    private static func json<T: Encodable>(_ value: T) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }
}
