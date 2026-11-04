import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class UserCabalsModelTests: XCTestCase {
    private func make(
        _ replies: [StubTransport.Reply], viewerID: String? = nil
    ) -> (UserCabalsModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "t"), transport: transport)
        return (UserCabalsModel(userID: "user-2", viewerID: viewerID, api: api), transport)
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

    private func cabals(shared count: Int, extra: Int = 0) -> Components.Schemas.UserCabals {
        let names = ["AC Capital", "Degens", "Big Ballers", "Sunday Investors", "Tech Titans"]
        let rows = (0..<(count + extra)).map { index in
            Components.Schemas.UserCabal(
                cabal: .init(id: "c\(index)", name: names[index], pictureUrl: index == 0 ? "https://x/0.png" : nil),
                memberCount: 4, role: .member, canVote: true, shared: index < count, ownershipBps: 100)
        }
        return .init(cabals: rows)
    }

    func testIsSharedFollowsTheAPIFlag() async throws {
        let (model, _) = make([try reply(.sample)])
        await model.load()
        XCTAssertEqual(model.rows.map(\.isShared), [true, true, false])
        XCTAssertEqual(model.sharedRows.map(\.name), ["AC Capital", "Degens"])
    }

    func testOneSharedCabalReadsBothIn() async throws {
        let (model, _) = make([try reply(cabals(shared: 1, extra: 1))])
        await model.load()
        XCTAssertEqual(model.sharedLine?.text, "Both in AC Capital")
        XCTAssertEqual(model.sharedLine?.names, ["AC Capital"])
        XCTAssertEqual(model.sharedLine?.moreCount, 0)
        XCTAssertEqual(model.sharedLine?.marks.map(\.pictureURL), ["https://x/0.png"])
    }

    func testTwoSharedCabalsNameBothAndIgnoreTheUnshared() async throws {
        let (model, _) = make([try reply(.sample)])
        await model.load()
        XCTAssertEqual(model.sharedLine?.text, "Both in AC Capital and Degens")
        XCTAssertEqual(model.sharedLine?.marks.map(\.name), ["AC Capital", "Degens"])
        XCTAssertEqual(model.sharedLine?.marks.map(\.id), model.sharedRows.map(\.id))
    }

    func testThreeOrMoreSharedCabalsCountTheRest() async throws {
        let (three, _) = make([try reply(cabals(shared: 3))])
        await three.load()
        XCTAssertEqual(three.sharedLine?.text, "In AC Capital, Degens and 1 more")
        XCTAssertEqual(three.sharedLine?.moreCount, 1)
        let (five, _) = make([try reply(cabals(shared: 5))])
        await five.load()
        XCTAssertEqual(five.sharedLine?.text, "In AC Capital, Degens and 3 more")
        XCTAssertEqual(five.sharedLine?.marks.count, 2)
    }

    func testNoSharedLineWithoutSharedRowsOrALoadedList() async throws {
        let (none, _) = make([try reply(cabals(shared: 0, extra: 2))])
        XCTAssertNil(none.sharedLine)
        await none.load()
        XCTAssertNil(none.sharedLine)
        let (failing, _) = make([.failure(URLError(.notConnectedToInternet))])
        await failing.load()
        XCTAssertNil(failing.sharedLine)
    }

    func testNoSharedLineOnTheViewersOwnProfile() async throws {
        let (own, _) = make([try reply(.sample)], viewerID: "user-2")
        await own.load()
        XCTAssertNil(own.sharedLine)
        let (other, _) = make([try reply(.sample)], viewerID: "user-1")
        await other.load()
        XCTAssertNotNil(other.sharedLine)
    }
}
