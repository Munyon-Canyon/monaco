import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ProposalSegmentsTests: XCTestCase {
    @MainActor
    func testTheExecutedSegmentPagesThroughTwentyFiveProposals() async throws {
        let first = (0..<20).map { Self.proposal("executed", id: "e\($0)") }.joined(separator: ",")
        let second = (20..<25).map { Self.proposal("executed", id: "e\($0)") }.joined(separator: ",")
        let transport = StubTransport(scripted: [
            .json(.ok, #"{"proposals":[\#(first)],"next_cursor":"next"}"#),
            .json(.ok, #"{"proposals":[\#(second)],"next_cursor":null}"#),
        ])
        let segments = Self.segments(transport)
        let executed = segments.model(for: .executed)

        await executed.load()
        await executed.pager.loadMore()

        XCTAssertEqual(executed.pager.items.count, 25)
        XCTAssertEqual(executed.pager.phase, .exhausted)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertTrue(paths.allSatisfy { $0.contains("filter=executed") }, "\(paths)")
        XCTAssertTrue(paths[1].contains("cursor=next"), "\(paths)")
    }

    @MainActor
    func testEachSegmentOwnsOneModelWithItsFilter() {
        let segments = Self.segments(StubTransport(.json(.ok, "{}")))
        for segment in ProposalSegment.allCases {
            XCTAssertTrue(segments.model(for: segment) === segments.model(for: segment))
            XCTAssertEqual(segments.model(for: segment).filter, segment.filter)
        }
        XCTAssertEqual(ProposalSegment.allCases.map(\.title), ["Open", "Passed", "Executed", "Failed"])
        XCTAssertEqual(ProposalSegment.executed.emptyTitle, "No executed proposals yet")
        XCTAssertEqual(segments.loaded.count, 4)
    }

    @MainActor
    func testAVoteThatClosesAProposalMovesItOutOfOpenAndIntoExecuted() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page([Self.proposal("open", id: "p")])),
            .json(.ok, Self.page([])),
            .json(.ok, Self.page([])),
            .json(.ok, Self.page([Self.proposal("executed", id: "p")])),
        ])
        let segments = Self.segments(transport)
        let open = segments.model(for: .open)
        let executed = segments.model(for: .executed)
        await open.load()
        await executed.load()
        XCTAssertEqual(open.pager.items.map(\.id), ["p"])

        await segments.refreshLoaded()

        XCTAssertEqual(open.pager.items.map(\.id), [])
        XCTAssertEqual(executed.pager.items.map(\.id), ["p"])
    }

    @MainActor
    func testARefreshDropsAProposalThatNoLongerMatchesTheSegment() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page([Self.proposal("open", id: "p"), Self.proposal("open", id: "q")])),
            .json(.ok, Self.page([Self.proposal("executed", id: "p"), Self.proposal("open", id: "q")])),
        ])
        let open = Self.segments(transport).model(for: .open)
        await open.load()

        await open.refresh()

        XCTAssertEqual(open.pager.items.map(\.id), ["q"])
    }

    @MainActor
    func testTheCabalSectionKeepsVotedOpenProposalsAndTitlesItByWhatIsNeeded() async throws {
        let transport = StubTransport(
            .json(
                .ok,
                Self.page([
                    Self.proposal("open", id: "voted", ballot: "yes"),
                    Self.proposal("open", id: "todo"),
                    Self.proposal("executed", id: "done", ballot: "no"),
                ])))
        let model = ProposalListModel(
            cabalID: "c", filter: .all, repository: Self.repository(transport), hints: FakeHintStream())
        await model.load()

        let needing = try XCTUnwrap(model.cabalSection(votedThisSession: []))
        XCTAssertEqual(needing.title, "Needs your vote")
        XCTAssertEqual(needing.count, 1)
        XCTAssertEqual(needing.proposals.map(\.id), ["todo", "voted"])

        let allVoted = try XCTUnwrap(model.cabalSection(votedThisSession: ["todo"]))
        XCTAssertEqual(allVoted.title, "Needs your vote")
        XCTAssertEqual(allVoted.proposals.map(\.id), ["todo", "voted"])
    }

    @MainActor
    func testTheCabalSectionIsTitledProposalsWhenNothingNeedsAVote() async throws {
        let transport = StubTransport(
            .json(.ok, Self.page([Self.proposal("open", id: "voted", ballot: "yes", canVote: true)])))
        let model = ProposalListModel(
            cabalID: "c", filter: .all, repository: Self.repository(transport), hints: FakeHintStream())
        await model.load()

        let section = try XCTUnwrap(model.cabalSection(votedThisSession: []))

        XCTAssertEqual(section.title, "Proposals")
        XCTAssertNil(section.count)
        XCTAssertEqual(section.proposals.map(\.id), ["voted"])
    }

    @MainActor
    func testTheCabalSectionIsAbsentWithoutOpenProposals() async throws {
        let transport = StubTransport(.json(.ok, Self.page([Self.proposal("executed", id: "x")])))
        let model = ProposalListModel(
            cabalID: "c", filter: .all, repository: Self.repository(transport), hints: FakeHintStream())
        await model.load()

        XCTAssertNil(model.cabalSection(votedThisSession: []))
    }

    func testListTakesCanVoteFromTheServerForAnOpenProposal() async throws {
        for canVote in [true, false] {
            let body = Self.page([Self.proposal("open", id: "p", canVote: canVote)])
            let result = try await Self.repository(StubTransport(.json(.ok, body))).list(
                cabalID: "c", filter: .open, cursor: nil)
            XCTAssertEqual(result.items.map(\.canVote), [canVote])
        }
    }

    func testEveryFilterIncludesExactlyItsStatuses() {
        let expected: [ProposalFilter: Set<ProposalStatus>] = [
            .open: [.open], .passed: [.passed], .executed: [.executed],
            .failed: [.failed, .expired, .executionBlocked],
            .closed: Set(ProposalStatus.allCases).subtracting([.open]),
            .all: Set(ProposalStatus.allCases),
        ]
        for (filter, statuses) in expected {
            XCTAssertEqual(Set(ProposalStatus.allCases.filter(filter.includes)), statuses, filter.rawValue)
        }
    }

    @MainActor
    private static func segments(_ transport: StubTransport) -> ProposalSegmentsModel {
        ProposalSegmentsModel(cabalID: "c", repository: repository(transport), hints: FakeHintStream())
    }

    private static func repository(_ transport: StubTransport) -> ProposalsRepository {
        ProposalsRepository(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport))
    }

    private static func page(_ items: [String]) -> String {
        #"{"proposals":[\#(items.joined(separator: ","))],"next_cursor":null}"#
    }

    private static func proposal(_ status: String, id: String, ballot: String? = nil, canVote: Bool = true) -> String {
        let mine = ballot.map { #""\#($0)""# } ?? "null"
        return
            #"{"id":"\#(id)","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"\#(status)","status_reason":null,"status_message":null,"expires_at":"2099-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":\#(mine),"can_vote":\#(canVote)}"#
    }
}
