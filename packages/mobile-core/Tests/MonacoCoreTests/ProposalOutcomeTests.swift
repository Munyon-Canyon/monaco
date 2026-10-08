import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ProposalOutcomeTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    @MainActor
    func testAVotedSellThatLeavesPassedForExecutedFiresOneSoldOutcome() async throws {
        let model = try await loaded([Self.sell("passed")], then: [Self.sell("executed")])
        var outcomes: [ProposalOutcome] = []
        model.onOutcome = { outcomes.append($0) }

        await model.refresh()

        XCTAssertEqual(outcomes.map(\.proposal.id), ["p"])
        XCTAssertEqual(outcomes.first?.toast(asset: Self.amazon), "Sold 0.2161 shares of Amazon")
    }

    @MainActor
    func testABlockedSellFiresCouldntSellWithTheStatusMessage() async throws {
        let blocked = Self.sell("execution_blocked", message: "The cabal holds less than that.")
        let model = try await loaded([Self.sell("passed")], then: [blocked])
        var outcomes: [ProposalOutcome] = []
        model.onOutcome = { outcomes.append($0) }

        await model.refresh()

        XCTAssertEqual(outcomes.first?.toast(asset: Self.amazon), "Couldn't sell: The cabal holds less than that.")
        XCTAssertEqual(
            ProposalOutcome(
                from: .passed, to: ProposalSummary(try Self.proposal(status: "execution_blocked", kind: "sell"))
            )?.toast(asset: nil), "Couldn't sell")
    }

    @MainActor
    func testNoOutcomeWithoutAVoteOrWithoutAPassedStartOrForABuy() async throws {
        var outcomes: [ProposalOutcome] = []
        for (before, after) in [
            (Self.sell("passed", ballot: nil), Self.sell("executed", ballot: nil)),
            (Self.sell("open"), Self.sell("executed")),
            (Self.sell("passed"), Self.sell("passed")),
            (Self.sell("passed", kind: "buy"), Self.sell("executed", kind: "buy")),
        ] {
            let model = try await loaded([before], then: [after])
            model.onOutcome = { outcomes.append($0) }
            await model.refresh()
        }
        XCTAssertEqual(outcomes, [])
    }

    @MainActor
    func testTheFirstLoadFiresNothingForAnAlreadyExecutedSell() async throws {
        let model = ProposalListModel(
            cabalID: "c", filter: .all, repository: Self.repository([Self.page([Self.sell("executed")])]),
            hints: FakeHintStream())
        var outcomes: [ProposalOutcome] = []
        model.onOutcome = { outcomes.append($0) }
        await model.load()
        XCTAssertEqual(outcomes, [])
    }

    @MainActor
    func testOutcomesStayOnTheCabalPageForTwentyFourHoursAfterClosing() async throws {
        let hour: TimeInterval = 3600
        let items = [
            Self.sell("executed", id: "fresh", expires: now.addingTimeInterval(-23 * hour)),
            Self.sell("execution_blocked", id: "blocked", expires: now.addingTimeInterval(-1 * hour)),
            Self.sell("executed", id: "stale", expires: now.addingTimeInterval(-25 * hour)),
            Self.sell("passed", id: "selling", expires: now.addingTimeInterval(-1 * hour)),
            Self.sell("open", id: "open", expires: now.addingTimeInterval(hour)),
        ]
        let model = ProposalListModel(
            cabalID: "c", filter: .all, repository: Self.repository([Self.page(items)]), hints: FakeHintStream())
        await model.load()

        XCTAssertEqual(model.recentOutcomes(now: now).map(\.id), ["fresh", "blocked"])
        XCTAssertEqual(ProposalChip.label(status: .executed, isSell: true), "Sold")
        XCTAssertEqual(ProposalChip.label(status: .executionBlocked, isSell: true), "Couldn't sell")
    }

    @MainActor
    func testAnOpenVoteResponseTallyReplacesTheGuessedCount() async throws {
        let response =
            #"{"proposal_id":"p","status":"open","tally":{"yes":3,"no":0,"voters":3,"needed":2},"my_ballot":"yes"}"#
        let voting = ProposalVoteModel(repository: Self.repository([response]))
        let summary = ProposalSummary(try Self.proposal(status: "open", ballot: nil))

        let sent = await voting.vote("yes", on: summary)

        XCTAssertTrue(sent)
        XCTAssertEqual(voting.applying(summary).tally, ProposalTally(yes: 3, no: 0, voters: 3, needed: 2))
    }

    @MainActor
    func testAClosingVoteResponseKeepsTheGuessAndLeavesTheTallyToTheRefresh() async throws {
        let response =
            #"{"proposal_id":"p","status":"passed","tally":{"yes":3,"no":0,"voters":3,"needed":2},"my_ballot":"yes"}"#
        let voting = ProposalVoteModel(repository: Self.repository([response]))
        let summary = ProposalSummary(try Self.proposal(status: "open", ballot: nil))

        _ = await voting.vote("yes", on: summary)

        XCTAssertEqual(voting.applying(summary).tally.yes, 2)
    }

    @MainActor
    private func loaded(_ first: [String], then second: [String]) async throws -> ProposalListModel {
        let model = ProposalListModel(
            cabalID: "c", filter: .all, repository: Self.repository([Self.page(first), Self.page(second)]),
            hints: FakeHintStream())
        await model.load()
        return model
    }

    private static let amazon: ProposalAsset = {
        var detail = Components.Schemas.AssetDetail.googl
        detail.displayName = "Amazon"
        detail.decimals = 6
        return ProposalAsset(detail)
    }()

    private static func repository(_ bodies: [String]) -> ProposalsRepository {
        let transport = StubTransport(scripted: bodies.map { .json(.ok, $0) })
        return ProposalsRepository(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport))
    }

    private static func page(_ items: [String]) -> String {
        #"{"proposals":[\#(items.joined(separator: ","))],"next_cursor":null}"#
    }

    private static func proposal(status: String, ballot: String? = "yes", kind: String = "buy")
        throws -> Components.Schemas.Proposal
    {
        let json = sell(status, ballot: ballot, kind: kind)
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(Components.Schemas.Proposal.self, from: Data(json.utf8))
    }

    private static func sell(
        _ status: String, id: String = "p", ballot: String? = "yes", kind: String = "sell", message: String? = nil,
        expires: Date = Date(timeIntervalSince1970: 4_000_000_000)
    ) -> String {
        let mine = ballot.map { #""\#($0)""# } ?? "null"
        let text = message.map { #""\#($0)""# } ?? "null"
        let reason = message == nil ? "null" : #""InsufficientFunds""#
        let expiry = ISO8601DateFormatter().string(from: expires)
        let tokens = kind == "sell" ? "216100" : "null"
        return
            #"{"id":"\#(id)","cabal_id":"c","proposer_id":"u","kind":"\#(kind)","symbol":"AMZNx","usdc_micros":null,"token_amount":\#(tokens),"quote_out_amount":1,"thesis":null,"status":"\#(status)","status_reason":\#(reason),"status_message":\#(text),"expires_at":"\#(expiry)","created_at":"2025-01-01T00:00:00Z","tally":{"yes":1,"no":0,"voters":3,"needed":2},"my_ballot":\#(mine),"can_vote":false}"#
    }
}
