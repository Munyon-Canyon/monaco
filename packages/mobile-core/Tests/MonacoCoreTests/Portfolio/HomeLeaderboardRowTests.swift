import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class HomeLeaderboardRowTests: XCTestCase {
    private static let callsPath = "/v1/leaderboards/calls?board=people&period=month"

    private var withoutMe: Components.Schemas.CallBoard {
        var board = Components.Schemas.CallBoard.peopleMonth
        board.me = nil
        board.above = nil
        board.below = nil
        board.gap = nil
        return board
    }

    private var viewerLeads: Components.Schemas.CallBoard {
        var board = Components.Schemas.CallBoard.peopleMonth
        board.top[0].isViewer = true
        return board
    }

    private func row(_ board: Components.Schemas.CallBoard) -> HomeLeaderboardRow? {
        HomeLeaderboardRow(board)
    }

    func testARankedMemberSeesTheLeaderTheirPlaceAndTheirRate() async throws {
        let shown = try XCTUnwrap(row(.peopleMonth))
        XCTAssertEqual(shown.title, "Christelle leads · 92%")
        XCTAssertEqual(shown.subtitle, "This month · you are #8 of 24")
        XCTAssertEqual(shown.rateText, "78%")
        XCTAssertEqual(shown.leaderName, "Christelle")
    }

    func testBelowFiveResolvedVotesThereIsNoRateAndTwoMoreToRank() async throws {
        let shown = try XCTUnwrap(row(.peopleWeekUnranked))
        XCTAssertEqual(shown.title, "Christelle leads · 100%")
        XCTAssertEqual(shown.subtitle, "This month · 2 more to rank")
        XCTAssertNil(shown.rateText)
    }

    func testAMemberWithNoRowAtAllStillSeesTheLeader() async throws {
        let shown = try XCTUnwrap(row(withoutMe))
        XCTAssertEqual(shown.subtitle, "This month · 24 ranked")
        XCTAssertNil(shown.rateText)
    }

    func testTheViewerWhoLeadsReadsYouLead() async throws {
        let shown = try XCTUnwrap(row(viewerLeads))
        XCTAssertEqual(shown.title, "You lead · 92%")
        XCTAssertEqual(shown.rateText, "78%")
    }

    func testNoBoardYetHidesTheBlock() async {
        XCTAssertNil(row(.empty))
    }

    func testAModelHidesWithNoBoardAndReadsTheMonthsPeopleBoardOnce() async throws {
        let transport = StubTransport(routes: [
            Self.callsPath: [.json(.ok, try Self.json(.empty))]
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let model = HomeLeaderboardModel(api: api, hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.phase, .hidden)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(sent.first?.path, Self.callsPath)
    }

    func testAModelShowsTheRowAndKeepsItWhenAReloadFails() async throws {
        let transport = StubTransport(routes: [
            Self.callsPath: [
                .json(.ok, try Self.json(.peopleMonth)), .json(.internalServerError, "{}"),
            ]
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let model = HomeLeaderboardModel(api: api, hints: FakeHintStream())
        await model.load()
        guard case .ready(let shown) = model.phase else { return XCTFail("not ready: \(model.phase)") }
        XCTAssertEqual(shown.rateText, "78%")
        XCTAssertEqual(model.readStatus, .settled)
        await model.load()
        guard case .ready = model.phase else { return XCTFail("lost the row: \(model.phase)") }
    }

    func testAFailedFirstReadIsFailedAndOfflineWhenTheNetworkIsDown() async {
        let transport = StubTransport(routes: [Self.callsPath: [.json(.internalServerError, "{}")]])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let model = HomeLeaderboardModel(api: api, hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.readStatus, .failed)
    }

    private static func json(_ board: Components.Schemas.CallBoard) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(board), as: UTF8.self)
    }
}
