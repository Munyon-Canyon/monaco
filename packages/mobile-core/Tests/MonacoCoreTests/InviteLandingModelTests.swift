import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

private final class MemoryStore: KeyValueStoring {
    var values: [String: Any] = [:]

    func data(forKey key: String) -> Data? { values[key] as? Data }
    func bool(forKey key: String) -> Bool { values[key] as? Bool ?? false }
    func set(_ value: Any?, forKey key: String) { values[key] = value }
    func removeObject(forKey key: String) { values[key] = nil }
}

private actor Counter {
    private(set) var value = 0
    func bump() -> Int {
        value += 1
        return value
    }
}

@MainActor
final class InviteLandingModelTests: XCTestCase {
    private static let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"
    private static let previewPath = "/v1/referrals/k7m4qx2p/cabals/\(cabalID)"
    private static let cabalPath = "/v1/cabals/\(cabalID)"
    private static let requestPath = "\(cabalPath)/access-requests"
    private static let memberPath = "\(cabalPath)/members"
    private let now = Date(timeIntervalSince1970: 1_793_200_000 - (4 * 3600 + 720))
    private let memory = MemoryStore()

    func testTheFrameReadsAsDrawn() async throws {
        let (model, _) = try make(routes: [Self.previewPath: [try Self.preview(.sample)]])
        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        let content = try XCTUnwrap(model.content)
        XCTAssertEqual(content.facts, "Cabal \u{00B7} 8 members \u{00B7} since Aug 2026")
        XCTAssertEqual(content.inviterLine, "Andres invited you")
        XCTAssertEqual(content.membersLine, "Christelle, Logan, Mia and 4 more")
        XCTAssertEqual(content.stats.treasury, "$20,000")
        XCTAssertEqual(content.stats.oneMonth, "\u{2212}4.1%")
        XCTAssertTrue(content.stats.oneMonthIsLoss)
        XCTAssertEqual(content.stats.turnout, "94%")
        let proposal = try XCTUnwrap(content.proposal)
        XCTAssertEqual(proposal.title, "Buy NVDA \u{00B7} $400")
        XCTAssertEqual(proposal.byline, "Mia \u{00B7} small add before earnings")
        XCTAssertEqual(proposal.yesText, "\u{2713} 4")
        XCTAssertEqual(proposal.noText, "1 \u{2715}")
        XCTAssertEqual(proposal.voteNote, "Join to vote")
        XCTAssertEqual(proposal.detail, "5 of 8 voted \u{00B7} 6 replies \u{00B7} ends in 4h")
        XCTAssertEqual(model.buttonTitle, "Ask to join AC Capital")
        XCTAssertTrue(model.buttonEnabled)
        XCTAssertFalse(model.showsAcceptDecline)
        XCTAssertEqual(model.note, "You sign in at your first vote or fund.")
    }

    func testNullMonthFactsShowDashes() async throws {
        let (model, _) = try make(routes: [Self.previewPath: [try Self.preview(.sampleMonthFactsNull)]])
        await model.load()

        let content = try XCTUnwrap(model.content)
        XCTAssertEqual(content.stats.oneMonth, "\u{2014}")
        XCTAssertFalse(content.stats.oneMonthIsLoss)
        XCTAssertEqual(content.stats.turnout, "\u{2014}")
        XCTAssertNil(content.proposal)
    }

    func testCopyEdges() {
        XCTAssertEqual(
            InviteLandingCopy.facts(memberCount: 1, since: "2026-01-02"),
            "Cabal \u{00B7} 1 member \u{00B7} since Jan 2026")
        XCTAssertEqual(InviteLandingCopy.monthYear("garbage"), "garbage")
        XCTAssertEqual(InviteLandingCopy.monthYear("2026-13-01"), "2026-13-01")
        XCTAssertNil(InviteLandingCopy.membersLine(names: [], moreCount: 0))
        XCTAssertEqual(InviteLandingCopy.membersLine(names: ["Mia Lee"], moreCount: 0), "Mia")
        XCTAssertEqual(InviteLandingCopy.membersLine(names: ["A", "B", "C"], moreCount: 0), "A, B and C")
        XCTAssertEqual(InviteLandingCopy.buttonTitle(name: "X", policy: .open), "Join X")
    }

    func testAnOpenCabalButtonSaysJoinSignedOut() async throws {
        var source = Components.Schemas.ReferralCabal.sample
        source.cabal?.joinMode = .open
        let (model, _) = try make(routes: [Self.previewPath: [try Self.preview(source)]])
        await model.load()
        XCTAssertEqual(model.buttonTitle, "Join AC Capital")
    }

    func testSignedOutSendsNoAuthorizationHeader() async throws {
        let (model, transport) = try make(routes: [Self.previewPath: [try Self.preview(.sample)]], token: nil)
        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), [Self.previewPath])
        XCTAssertNil(sent[0].headerFields[.authorization])
    }

    func testANullCabalRedirectsToTheReferrer() async throws {
        let (model, _) = try make(routes: [Self.previewPath: [try Self.preview(.sampleMiss)]])
        await model.load()
        XCTAssertEqual(model.redirect, .referrer("k7m4qx2p"))
        XCTAssertNil(model.content)
        XCTAssertFalse(model.buttonEnabled)
    }

    func testAFailedFirstLoadFailsAndALaterLoadRecovers() async throws {
        let (model, _) = try make(routes: [
            Self.previewPath: [.failure(URLError(.notConnectedToInternet)), try Self.preview(.sample)]
        ])
        await model.load()
        XCTAssertEqual(model.phase, .failed)
        await model.load()
        XCTAssertEqual(model.phase, .loaded)
    }

    func testAFailedRefreshKeepsThePreview() async throws {
        let (model, _) = try make(routes: [
            Self.previewPath: [try Self.preview(.sample), .failure(URLError(.timedOut))]
        ])
        await model.load()
        await model.load()
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertNotNil(model.preview)
    }

    func testJoinSignedOutSavesAJoinTargetAndHandsToSignIn() async throws {
        let (model, _) = try make(routes: [Self.previewPath: [try Self.preview(.sample)]])
        await model.join()
        XCTAssertNil(model.redirect)

        await model.load()
        await model.join()

        XCTAssertEqual(model.redirect, .signIn)
        let saved = PendingLinkTargetStore(store: memory).peek(now: now)
        XCTAssertEqual(
            saved,
            PendingLinkTarget(
                kind: .join, code: "k7m4qx2p", cabalID: Self.cabalID, proposalID: nil, joinTapped: true, savedAt: now))
    }

    func testSignedInJoinBeforeLoadDoesNothing() async throws {
        let (model, transport) = try make(routes: [:], viewerID: "me")
        await model.join()
        let count = await transport.sent.count
        XCTAssertEqual(count, 0)
    }
}

@MainActor
extension InviteLandingModelTests {
    func testAMemberGoesStraightToTheCabalAndDoesNotJoinAgain() async throws {
        store(joinTapped: true)
        let (model, transport) = try make(
            routes: Self.routes(try Self.cabal(role: "member")), viewerID: "me")
        await model.load()

        XCTAssertEqual(model.redirect, .cabal(Self.cabalID))
        XCTAssertNil(PendingLinkTargetStore(store: memory).peek(now: now))
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths, [Self.previewPath, Self.cabalPath])
    }

    func testAFreshJoinOfAnOpenCabalRedirectsToTheCabal() async throws {
        let (model, transport) = try make(
            routes: Self.routes(
                try Self.cabal(mode: "open"),
                [Self.memberPath: [.json(.ok, try Self.json(Components.Schemas.Cabal.sample(role: "member")))]],
                after: try Self.cabal(role: "member")),
            viewerID: "me")
        await model.load()
        XCTAssertEqual(model.buttonTitle, "Join AC Capital")
        XCTAssertTrue(model.buttonEnabled)

        await model.join()

        XCTAssertEqual(model.redirect, .cabal(Self.cabalID))
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertTrue(paths.contains(Self.memberPath))
        XCTAssertFalse(paths.contains(Self.requestPath))
    }

    func testARequestCabalSendsViaCodeAndShowsRequestSent() async throws {
        let pending = try Self.cabal(request: ("r1", "request"))
        let (model, transport) = try make(
            routes: Self.routes(try Self.cabal(), [Self.requestPath: [Self.requested]], after: pending), viewerID: "me")
        await model.load()
        XCTAssertEqual(model.buttonTitle, "Ask to join AC Capital")

        await model.join()

        XCTAssertEqual(model.toast, "Request sent to AC Capital.")
        XCTAssertEqual(model.buttonTitle, "Request sent")
        XCTAssertFalse(model.buttonEnabled)
        let sent = await transport.sent
        let bodies = await transport.sentBodies
        let index = try XCTUnwrap(sent.firstIndex { $0.path == Self.requestPath })
        let body = try JSONSerialization.jsonObject(with: try XCTUnwrap(bodies[index])) as? [String: String]
        XCTAssertEqual(body, ["via_code": "k7m4qx2p"])
        XCTAssertNil(model.redirect)
    }

    func testAnAlreadyPendingRequestShowsItIsWaiting() async throws {
        let pending = try Self.cabal(request: ("r1", "request"))
        let (model, _) = try make(
            routes: Self.routes(
                try Self.cabal(), [Self.requestPath: [Self.problem(409, "request_pending")]], after: pending),
            viewerID: "me")
        await model.load()
        await model.join()
        XCTAssertEqual(model.toast, "Your request is waiting.")
    }

    func testABannedCabalSaysItIsNotTakingMembers() async throws {
        let (model, _) = try make(
            routes: Self.routes(try Self.cabal(), [Self.requestPath: [Self.problem(409, "cabal_banned")]]),
            viewerID: "me")
        await model.load()
        await model.join()
        XCTAssertEqual(model.toast, "This cabal isn't taking members.")
    }

    func testAnotherRefusalShowsTheServerMessage() async throws {
        let (model, _) = try make(
            routes: Self.routes(try Self.cabal(), [Self.requestPath: [Self.problem(403, "forbidden")]]), viewerID: "me")
        await model.load()
        await model.join()
        XCTAssertEqual(model.toast, "message forbidden")
    }

    func testAnExistingInvitationShowsAcceptAndDeclineAndNeverEnters() async throws {
        let invited = try Self.cabal(request: ("i1", "invite"))
        let (model, transport) = try make(routes: Self.routes(invited), viewerID: "me")
        await model.load()

        XCTAssertTrue(model.showsAcceptDecline)
        XCTAssertFalse(model.buttonEnabled)
        await model.join()
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertFalse(paths.contains(Self.requestPath))
        XCTAssertFalse(paths.contains(Self.memberPath))
    }

    func testAcceptJoinsAndRedirects() async throws {
        let invited = try Self.cabal(request: ("i1", "invite"))
        let (model, _) = try make(
            routes: Self.routes(
                invited,
                [
                    "\(Self.requestPath)/i1/decision": [
                        .json(.ok, #"{"id":"i1","direction":"invite","status":"approved"}"#)
                    ]
                ]),
            viewerID: "me")
        await model.load()
        await model.accept()
        XCTAssertEqual(model.redirect, .cabal(Self.cabalID))
    }

    func testDeclineShowsItsToast() async throws {
        let invited = try Self.cabal(request: ("i1", "invite"))
        let (model, _) = try make(
            routes: Self.routes(
                invited,
                [
                    "\(Self.requestPath)/i1/decision": [
                        .json(.ok, #"{"id":"i1","direction":"invite","status":"denied"}"#)
                    ]
                ],
                after: try Self.cabal()),
            viewerID: "me")
        await model.load()
        await model.decline()
        XCTAssertEqual(model.toast, "Invite declined.")
        XCTAssertNil(model.redirect)
    }

    func testAcceptFailureShowsAToastAndStays() async throws {
        let invited = try Self.cabal(request: ("i1", "invite"))
        let (model, _) = try make(
            routes: Self.routes(invited, ["\(Self.requestPath)/i1/decision": [Self.problem(403, "forbidden")]]),
            viewerID: "me")
        await model.load()
        await model.accept()
        XCTAssertEqual(model.toast, "message forbidden")
        XCTAssertNil(model.redirect)
    }

    func testAcceptWithoutAnInvitationDoesNothing() async throws {
        let (model, transport) = try make(routes: Self.routes(try Self.cabal()), viewerID: "me")
        await model.load()
        await model.accept()
        await model.decline()
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testTheFirstSignedInLoadWithASavedTargetJoinsOnce() async throws {
        store(joinTapped: true)
        let (model, transport) = try make(
            routes: Self.routes(
                try Self.cabal(mode: "open"),
                [Self.memberPath: [.json(.ok, try Self.json(Components.Schemas.Cabal.sample(role: "member")))]]),
            viewerID: "me")
        await model.load()
        XCTAssertEqual(model.redirect, .cabal(Self.cabalID))

        await model.load()

        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths.filter { $0 == Self.memberPath }.count, 1)
    }

    func testASavedTargetForARequestCabalSendsOneRequest() async throws {
        store(joinTapped: true)
        let pending = try Self.cabal(request: ("r1", "request"))
        let (model, _) = try make(
            routes: Self.routes(try Self.cabal(), [Self.requestPath: [Self.requested]], after: pending), viewerID: "me")
        await model.load()
        XCTAssertEqual(model.buttonTitle, "Request sent")
        XCTAssertEqual(model.toast, "Request sent to AC Capital.")
    }

    func testASavedTargetWithAPendingRequestDoesNotJoin() async throws {
        store(joinTapped: true)
        let pending = try Self.cabal(request: ("r1", "request"))
        let (model, transport) = try make(routes: Self.routes(pending), viewerID: "me")
        await model.load()
        XCTAssertEqual(model.buttonTitle, "Request sent")
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertFalse(paths.contains(Self.requestPath))
    }

    func testAFailedCabalReadToastsAndLeavesJoinDisabled() async throws {
        let (model, _) = try make(
            routes: [Self.previewPath: [try Self.preview(.sample)], Self.cabalPath: [.failure(URLError(.timedOut))]],
            viewerID: "me")
        await model.load()
        XCTAssertEqual(model.toast, "You're offline. Try again.")
        XCTAssertFalse(model.buttonEnabled)
    }

    func testSignedOutRunLoadsThenRefreshesEverySixtySeconds() async throws {
        let counter = Counter()
        let (model, transport) = try make(
            routes: [
                Self.previewPath: [try Self.preview(.sample), try Self.preview(.sample), try Self.preview(.sample)]
            ],
            sleep: { duration in
                XCTAssertEqual(duration, .seconds(60))
                if await counter.bump() > 2 { throw CancellationError() }
            })
        await model.run()
        let count = await transport.sent.count
        XCTAssertEqual(count, 3)
    }

    func testSignedInRunReloadsOnAProposalUpdateAndResync() async throws {
        let hints = FakeHintStream()
        let routes = Self.routes(try Self.cabal())
        let (model, transport) = try make(routes: routes, viewerID: "me", hints: hints)
        let task = Task { await model.run() }
        await hints.waitForSubscribers(1)
        await transport.waitForRequests(2)

        await hints.send(.changed(.cabal(Self.cabalID), what: "proposal_updated", id: "1"))
        await transport.waitForRequests(4)
        await hints.send(.resync)
        await transport.waitForRequests(6)
        task.cancel()
    }

    func testSignedInRunWithoutHintsStopsAfterTheLoad() async throws {
        let (model, transport) = try make(routes: Self.routes(try Self.cabal()), viewerID: "me")
        await model.run()
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testRunAfterALoadDoesNotLoadAgainSignedIn() async throws {
        let (model, transport) = try make(routes: Self.routes(try Self.cabal()), viewerID: "me")
        await model.load()
        await model.run()
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }
}

@MainActor
extension InviteLandingModelTests {
    private func store(joinTapped: Bool) {
        PendingLinkTargetStore(store: memory).save(
            PendingLinkTarget(
                kind: .join, code: "k7m4qx2p", cabalID: Self.cabalID, proposalID: nil, joinTapped: joinTapped,
                savedAt: now))
    }

    private func make(
        routes: [String: [StubTransport.Reply]], viewerID: String? = nil, token: String? = "token-1",
        hints: FakeHintStream? = nil,
        sleep: @escaping @Sendable (Duration) async throws -> Void = { _ in }
    ) throws -> (InviteLandingModel, StubTransport) {
        let transport = StubTransport(routes: routes)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: token), transport: transport)
        let model = InviteLandingModel(
            code: "k7m4qx2p", cabalID: Self.cabalID, api: api, hints: hints, viewerID: viewerID,
            pendingStore: PendingLinkTargetStore(store: memory), now: { [now] in now }, sleep: sleep)
        return (model, transport)
    }

    private static func routes(
        _ cabal: Components.Schemas.Cabal, _ extra: [String: [StubTransport.Reply]] = [:],
        after: Components.Schemas.Cabal? = nil
    ) -> [String: [StubTransport.Reply]] {
        var routes = extra
        var cabals: [StubTransport.Reply] = [.json(.ok, (try? json(cabal)) ?? "")]
        if let after { cabals.append(.json(.ok, (try? json(after)) ?? "")) }
        routes[cabalPath] = cabals + Array(repeating: cabals.last ?? .json(.ok, ""), count: 8)
        routes[previewPath] = Array(repeating: (try? preview(.sample)) ?? .json(.ok, ""), count: 10)
        return routes
    }

    private static let requested = StubTransport.Reply.json(
        .created, #"{"id":"r1","direction":"request","status":"pending"}"#)

    private static func cabal(
        role: String? = nil, mode: String = "request", request: (String, String)? = nil
    ) throws -> Components.Schemas.Cabal {
        var cabal = Components.Schemas.Cabal.sample(role: role)
        cabal.rules.joinMode = mode
        cabal.myAccessRequest = request.map { .init(id: $0.0, direction: $0.1, status: "pending") }
        return cabal
    }

    private static func preview(_ source: Components.Schemas.ReferralCabal) throws -> StubTransport.Reply {
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        response.headerFields[.cacheControl] = "no-cache"
        return .response(response, Data(try previewJSON(source).utf8))
    }

    private static func previewJSON(_ source: Components.Schemas.ReferralCabal) throws -> String {
        try json(source)
    }

    private static func json(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private static func problem(_ status: Int, _ code: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"message \#(code)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8))
    }
}
