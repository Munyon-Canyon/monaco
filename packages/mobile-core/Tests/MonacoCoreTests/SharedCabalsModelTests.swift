import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class SharedCabalsModelTests: XCTestCase {
    func testLoadReadsTheSharedCabalsOfThatUser() async throws {
        let (model, transport, _) = try make([.shared(.sample)])

        await model.load()

        XCTAssertEqual(model.summary?.rows.map(\.name), ["Alpha", "Sunday Investors"])
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/users/user-2/shared-cabals"])
    }

    func testARowCarriesPotFiguresAndNoStake() async throws {
        let (model, _, _) = try make([.shared(.sample)])

        await model.load()

        let rows = try XCTUnwrap(model.summary?.rows)
        XCTAssertEqual(rows[0].potValue, "$950.69")
        XCTAssertEqual(rows[0].returnText, "+1.49%")
        XCTAssertEqual(rows[1].returnText, "—")
    }

    func testNoSharedCabalsIsEmptyAndNamesTheOtherPerson() async throws {
        let (model, _, _) = try make([.shared(.sampleEmpty)])

        await model.load()

        XCTAssertEqual(model.summary?.isEmpty, true)
        XCTAssertEqual(
            SharedCabalsSummary.emptyLine(displayName: "Maya"), "You and Maya aren't in a cabal together yet.")
        XCTAssertEqual(
            SharedCabalsSummary.emptyLine(displayName: nil), "You and this person aren't in a cabal together yet.")
        XCTAssertEqual(
            SharedCabalsSummary.emptyLine(displayName: ""), "You and this person aren't in a cabal together yet.")
    }

    func testAFailedFirstLoadIsTheErrorStateAndARefreshFailureToasts() async throws {
        let (failing, _, _) = try make([.failure])
        await failing.load()
        guard case .failed(.transport) = failing.state else { return XCTFail("want a transport failure") }
        XCTAssertNil(failing.toast)

        let (model, _, _) = try make([.shared(.sample), .failure])
        await model.load()
        await model.load()

        XCTAssertNotNil(model.summary)
        XCTAssertEqual(model.toast, "You're offline. Try again.")
    }

    func testALeaderboardsUpdatedHintReadsAgain() async throws {
        let (model, transport, hints) = try make([.shared(.sampleEmpty), .shared(.sample)])
        await model.load()
        let task = Task { await model.observe() }
        while await hints.subscriberCount < 1 { await Task.yield() }

        await hints.send(.changed(.global, what: "leaderboards_updated", id: "1"))
        await transport.waitForRequests(2)
        while model.summary?.isEmpty != false { await Task.yield() }

        XCTAssertEqual(model.summary?.rows.count, 2)
        task.cancel()
    }

    private enum Script {
        case shared(Components.Schemas.SharedCabals)
        case failure

        func reply() throws -> StubTransport.Reply {
            switch self {
            case .shared(let shared):
                let encoder = JSONEncoder()
                encoder.dateEncodingStrategy = .iso8601
                return .json(.ok, String(decoding: try encoder.encode(shared), as: UTF8.self))
            case .failure:
                return .failure(URLError(.notConnectedToInternet))
            }
        }
    }

    private func make(_ script: [Script]) throws -> (SharedCabalsModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: try script.map { try $0.reply() })
        let hints = FakeHintStream()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (SharedCabalsModel(userID: "user-2", api: api, hints: hints), transport, hints)
    }
}
