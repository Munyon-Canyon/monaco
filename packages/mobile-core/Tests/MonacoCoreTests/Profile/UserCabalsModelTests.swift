import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class UserCabalsModelTests: XCTestCase {
    private func make(_ replies: [StubTransport.Reply]) -> (UserCabalsModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "t"), transport: transport)
        return (UserCabalsModel(userID: "user-2", api: api), transport)
    }

    private func reply(_ cabals: Components.Schemas.UserCabals) throws -> StubTransport.Reply {
        .json(.ok, String(decoding: try JSONEncoder().encode(cabals), as: UTF8.self))
    }

    func testLoadReadsThatUsersCabalsInServerOrder() async throws {
        let (model, transport) = make([try reply(.sample)])

        await model.load()

        XCTAssertEqual(model.rows.map(\.name), ["AC Capital", "Degens", "Big Ballers"])
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/users/user-2/cabals"])
    }

    func testTheSubLineNamesTheSharedCabalsAndTheMemberCountOfTheOthers() async throws {
        let (model, _) = make([try reply(.sample)])

        await model.load()

        XCTAssertEqual(
            model.rows.map(\.subtitle),
            [
                "You are both in \u{00B7} admin", "You are both in \u{00B7} voter", "12 members \u{00B7} voter",
            ])
        XCTAssertEqual(model.rows.map(\.ownership), ["20%", "15%", "8%"])
    }

    func testAMemberWhoCannotVoteIsAMember() async throws {
        let cabal = Components.Schemas.UserCabal(
            cabal: .sampleBigBallers, memberCount: 1, role: .member, canVote: false, shared: false, ownershipBps: 0)
        let (model, _) = make([try reply(.init(cabals: [cabal]))])

        await model.load()

        XCTAssertEqual(model.rows.map(\.subtitle), ["1 members \u{00B7} member"])
        XCTAssertEqual(model.rows.map(\.ownership), ["0%"])
    }

    func testHiddenPerformanceLeavesNoPercents() async throws {
        let (model, _) = make([try reply(.sampleHiddenPerformance)])

        await model.load()

        XCTAssertEqual(model.rows.count, 3)
        XCTAssertEqual(model.rows.map(\.ownership), [nil, nil, nil])
    }

    func testNoCabalsStillLoadsAnEmptyList() async throws {
        let (model, _) = make([try reply(.init(cabals: []))])

        await model.load()

        guard case .loaded(let rows) = model.state else { return XCTFail("want a loaded state") }
        XCTAssertTrue(rows.isEmpty)
    }

    func testAFailedFirstLoadFailsAndAFailedReloadKeepsTheRows() async throws {
        let (failing, _) = make([.failure(URLError(.notConnectedToInternet))])
        await failing.load()
        guard case .failed = failing.state else { return XCTFail("want a failed state") }
        XCTAssertTrue(failing.rows.isEmpty)

        let (model, _) = make([try reply(.sample), .failure(URLError(.notConnectedToInternet))])
        await model.load()
        await model.load()
        XCTAssertEqual(model.rows.count, 3)
    }
}
