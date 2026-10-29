import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CabalRuleProposalModelTests: XCTestCase {
    private func cabal(canPropose: Bool = true) -> Components.Schemas.Cabal {
        var cabal = Components.Schemas.Cabal.sampleWithMembers(role: "creator")
        cabal.me = .init(role: "creator", canVote: true, canPropose: canPropose)
        cabal.voterCount = 3
        cabal.rules.threshold = "majority"
        cabal.rules.maxTradeBps = nil
        cabal.members[2].canVote = false
        cabal.admins = [
            .init(userId: cabal.creator.userId, handle: "kai", displayName: "Kai", photoUrl: nil),
            .init(userId: Self.jordanID, handle: "jordan", displayName: "Jordan", photoUrl: nil),
        ]
        return cabal
    }

    private static let jordanID = "01890a5d-ac96-774b-bcce-b302099a8061"
    private static let priyaID = "01890a5d-ac96-774b-bcce-b302099a8062"

    private func make(
        _ change: CabalRuleProposalModel.Change, replies: [StubTransport.Reply] = [], canPropose: Bool = true
    ) -> (CabalRuleProposalModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        return (CabalRuleProposalModel(cabal: cabal(canPropose: canPropose), change: change, api: api), transport)
    }

    private func created() throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var detail = Components.Schemas.ProposalDetail.ruleSample(now: .now)
        detail.id = "rule-1"
        return .json(.created, String(decoding: try encoder.encode(detail), as: UTF8.self))
    }

    private func body(_ transport: StubTransport) async throws -> NSDictionary {
        let bodies = await transport.sentBodies
        let data = try XCTUnwrap(bodies.last ?? nil)
        return try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? NSDictionary)
    }

    func testShowSidesIsPostedOnceWithAnIdempotencyKey() async throws {
        let (model, transport) = make(.showSides(true), replies: [try created()])

        let outcome = await model.propose()

        XCTAssertEqual(outcome, .proposed(proposalID: "rule-1"))
        XCTAssertEqual(outcome.toast, "Proposed. Your cabal votes on it.")
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(sent.first?.method, .post)
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(model.cabal.id)/rule-proposals")
        let keyHeader = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertNotNil(sent.first?.headerFields[keyHeader])
        let json = try await body(transport)
        XCTAssertEqual(json, ["change": ["rule": "show_sides", "to": true]])
        XCTAssertFalse(model.isSubmitting)
    }

    func testTheReasonRidesAlongTrimmedAndCapped() async throws {
        let (model, transport) = make(.showSides(true), replies: [try created()])
        model.thesis = "  " + String(repeating: "a", count: 400)

        XCTAssertEqual(model.thesis.count, CabalRuleProposalModel.thesisLimit)
        _ = await model.propose()

        let json = try await body(transport)
        XCTAssertEqual((json["thesis"] as? String)?.count, CabalRuleProposalModel.thesisLimit - 2)
    }

    func testAVoterChangeCarriesTheUserID() async throws {
        let (model, transport) = make(.voter(userID: Self.priyaID, canVote: true), replies: [try created()])

        _ = await model.propose()

        let json = try await body(transport)
        XCTAssertEqual(json, ["change": ["rule": "voter", "to": true, "user_id": Self.priyaID]])
    }

    func testAnAdminChangeCarriesTheUserID() async throws {
        let (model, transport) = make(.admin(userID: Self.jordanID, isAdmin: false), replies: [try created()])

        _ = await model.propose()

        let json = try await body(transport)
        XCTAssertEqual(json, ["change": ["rule": "admin", "to": false, "user_id": Self.jordanID]])
    }

    func testTheValuedRulesUseTheirWireValues() async throws {
        let cases: [(CabalRuleProposalModel.Change, NSDictionary)] = [
            (.threshold(.unanimous), ["rule": "threshold", "to": "unanimous"]),
            (.window(.oneHour), ["rule": "proposal_expiry_seconds", "to": 3600]),
            (.maxTrade(2_000), ["rule": "max_trade_bps", "to": 2000]),
            (.maxTrade(nil), ["rule": "max_trade_bps", "to": NSNull()]),
        ]
        for (change, expected) in cases {
            let transport = StubTransport(scripted: [try created()])
            var withCap = cabal()
            withCap.rules.maxTradeBps = 3_000
            let api = APIClient(
                serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
            let capped = CabalRuleProposalModel(cabal: withCap, change: change, api: api)

            let outcome = await capped.propose()

            XCTAssertEqual(outcome, .proposed(proposalID: "rule-1"), "\(change)")
            let json = try await body(transport)
            XCTAssertEqual(json["change"] as? NSDictionary, expected, "\(change)")
        }
    }

    func testRefusalsNeverReachTheNetwork() async {
        let creatorID = cabal().creator.userId
        let refused: [(CabalRuleProposalModel.Change, CabalRuleProposalModel.Refusal)] = [
            (.voter(userID: creatorID, canVote: false), .targetsCreator),
            (.admin(userID: creatorID, isAdmin: false), .targetsCreator),
            (.voter(userID: "ghost", canVote: true), .notAMember),
            (.admin(userID: "ghost", isAdmin: true), .notAMember),
            (.admin(userID: Self.priyaID, isAdmin: true), .notAVoter),
            (.voter(userID: Self.jordanID, canVote: false), .adminMustVote),
            (.voter(userID: Self.jordanID, canVote: true), .unchanged),
            (.admin(userID: Self.jordanID, isAdmin: true), .unchanged),
            (.admin(userID: Self.priyaID, isAdmin: false), .unchanged),
            (.showSides(false), .unchanged),
            (.threshold(.majority), .unchanged),
            (.window(.oneDay), .unchanged),
            (.maxTrade(nil), .unchanged),
        ]
        for (change, refusal) in refused {
            let (model, transport) = make(change)
            XCTAssertEqual(model.refusal, refusal, "\(change)")
            XCTAssertFalse(model.canSubmit)

            let outcome = await model.propose()

            XCTAssertEqual(outcome, .refused(refusal), "\(change)")
            XCTAssertEqual(outcome.toast, refusal.message)
            let count = await transport.sent.count
            XCTAssertEqual(count, 0, "\(change)")
        }
    }

    func testTheLastAdminCannotBeRemoved() async {
        var single = cabal()
        single.admins = [.init(userId: Self.jordanID, handle: "jordan", displayName: "Jordan", photoUrl: nil)]
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: StubTransport(scripted: []))
        let model = CabalRuleProposalModel(
            cabal: single, change: .admin(userID: Self.jordanID, isAdmin: false), api: api)

        XCTAssertEqual(model.refusal, .lastAdmin)
        let outcome = await model.propose()
        XCTAssertEqual(outcome, .refused(.lastAdmin))
    }

    func testAMemberWhoCannotProposeIsRefused() async {
        let (model, transport) = make(.showSides(true), canPropose: false)

        XCTAssertFalse(model.canPropose)
        let outcome = await model.propose()

        XCTAssertEqual(outcome, .refused(.cannotPropose))
        XCTAssertEqual(outcome.toast, "Voters propose rule changes.")
        let count = await transport.sent.count
        XCTAssertEqual(count, 0)
    }

    func testARuleVoteAlreadyOpenMapsToItsToast() async throws {
        let problem =
            #"{"type":"about:blank","title":"Conflict","status":409,"code":"rule_change_open","#
            + #""message":"open","trace_id":"00000000000000000000000000000000","retryable":false}"#
        let (model, _) = make(
            .showSides(true),
            replies: [.response(status: .conflict, contentType: "application/problem+json", body: Data(problem.utf8))])

        let outcome = await model.propose()

        XCTAssertEqual(outcome, .alreadyOpen)
        XCTAssertEqual(outcome.toast, "A vote on this rule is already open.")
        XCTAssertFalse(model.isSubmitting)
    }

    func testAnOfflineProposeFailsWithTheOfflineToastAndRetriesWithTheSameKey() async throws {
        let (model, transport) = make(
            .showSides(true), replies: [.failure(URLError(.notConnectedToInternet)), try created()])

        let first = await model.propose()
        let second = await model.propose()

        XCTAssertEqual(first, .failed("You're offline. Try again."))
        XCTAssertEqual(first.toast, "You're offline. Try again.")
        XCTAssertEqual(second, .proposed(proposalID: "rule-1"))
        let keyHeader = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.map { $0.headerFields[keyHeader] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testThePassLineCountsTheVotersNeeded() {
        let (model, _) = make(.showSides(true))
        XCTAssertEqual(model.passLine, "Your cabal votes on it like a trade: 2 of 3 to pass.")
    }

    func testMaxTradeChoicesRunFromNoLimitToFiftyPercent() {
        XCTAssertEqual(CabalRuleProposalModel.maxTradeChoices, [nil, 1_000, 2_000, 3_000, 4_000, 5_000])
    }
}
