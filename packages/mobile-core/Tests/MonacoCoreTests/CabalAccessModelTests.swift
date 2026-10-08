import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalAccessModelTests: XCTestCase {
    private let requests = Components.Schemas.CabalAccessRequest.samples

    func testANonMemberCanRequestToJoin() async throws {
        let model = makeModel(StubTransport(.json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request")))))
        await model.load()
        XCTAssertEqual(model.standing, .join)
    }

    func testAPendingRequestOffersCancel() async throws {
        let cabal = Self.cabal(role: nil, mode: "request", request: ("r1", "request"))
        let model = makeModel(StubTransport(.json(.ok, try Self.encode(cabal))))
        await model.load()
        XCTAssertEqual(model.standing, .requested(requestID: "r1"))
    }

    func testAPendingInviteLeavesTheSlotToTheInbox() async throws {
        let cabal = Self.cabal(role: nil, mode: "request", request: ("i1", "invite"))
        let model = makeModel(StubTransport(.json(.ok, try Self.encode(cabal))))
        await model.load()
        XCTAssertEqual(model.standing, .hidden)
    }

    func testAMemberWhoIsNotTheCreatorSeesNothing() async throws {
        let transport = StubTransport(.json(.ok, try Self.encode(Self.cabal(role: "member", mode: "request"))))
        let model = makeModel(transport)
        await model.load()
        XCTAssertEqual(model.standing, .hidden)
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testTheCreatorWithNoRequestsSeesNothing() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: "creator", mode: "request"))),
            .json(.ok, "[]"),
        ])
        let model = makeModel(transport)
        await model.load()
        XCTAssertEqual(model.standing, .hidden)
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths.last?.hasPrefix("/v1/cabals/\(Self.cabalID)/access-requests"), true)
    }

    func testTheCreatorSeesEachPendingRequestByName() async throws {
        let model = try await creatorModel(StubTransport(scripted: try creatorReplies()))
        guard case .pending(let pending) = model.standing else { return XCTFail("got \(model.standing)") }
        XCTAssertEqual(pending.map(\.name), ["Jordan", "@priya"])
        XCTAssertEqual(CabalAccessStanding.heading(count: pending.count), "2 people want to join")
        XCTAssertEqual(CabalAccessStanding.heading(count: 1), "1 person wants to join")
    }

    func testApprovingDropsTheRowAndToastsApproved() async throws {
        let transport = StubTransport(
            scripted: try creatorReplies() + [
                .json(.ok, #"{"id":"r","direction":"request","status":"approved"}"#),
                .json(.ok, try Self.encode(Self.cabal(role: "creator", mode: "request"))),
                .json(.ok, try Self.encode([requests[1]])),
            ])
        let model = try await creatorModel(transport)
        guard case .pending(let pending) = model.standing else { return XCTFail("got \(model.standing)") }
        await model.decide(pending[0], approve: true)
        XCTAssertEqual(model.toast?.message, "Approved.")
        guard case .pending(let rest) = model.standing else { return XCTFail("got \(model.standing)") }
        XCTAssertEqual(rest.map(\.name), ["@priya"])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies[2])
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["decision": "approve"])
    }

    func testDenyingTheLastRequestHidesTheSlot() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: "creator", mode: "request"))),
            .json(.ok, try Self.encode([requests[0]])),
            .json(.ok, #"{"id":"r","direction":"request","status":"denied"}"#),
            .json(.ok, try Self.encode(Self.cabal(role: "creator", mode: "request"))),
            .json(.ok, "[]"),
        ])
        let model = makeModel(transport)
        await model.load()
        guard case .pending(let pending) = model.standing else { return XCTFail("got \(model.standing)") }
        await model.decide(pending[0], approve: false)
        XCTAssertEqual(model.toast?.message, "Denied.")
        XCTAssertEqual(model.standing, .hidden)
    }

    func testARequestAnsweredElsewhereLeavesWithTheServerMessage() async throws {
        let transport = StubTransport(
            scripted: try creatorReplies() + [
                Self.problem(409, "access_request_not_pending", "Someone already answered."),
                .json(.ok, try Self.encode(Self.cabal(role: "creator", mode: "request"))),
                .json(.ok, try Self.encode([requests[1]])),
            ])
        let model = try await creatorModel(transport)
        guard case .pending(let pending) = model.standing else { return XCTFail("got \(model.standing)") }
        await model.decide(pending[0], approve: true)
        XCTAssertEqual(model.toast?.message, "Someone already answered.")
        XCTAssertEqual(model.toast?.isSuccess, false)
    }

    func testRequestingShowsRequestSent() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request"))),
            .json(.created, #"{"id":"r1","direction":"request","status":"pending"}"#),
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request", request: ("r1", "request")))),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.enter()
        XCTAssertEqual(model.toast?.message, CabalEntry.requestedToast)
        XCTAssertEqual(model.standing, .requested(requestID: "r1"))
    }

    func testAskingToJoinShowsRequestSentAndStaysOutsideTheCabal() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request"))),
            .json(.created, #"{"id":"r1","direction":"request","status":"pending"}"#),
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request", request: ("r1", "request")))),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.enter()
        XCTAssertEqual(model.toast?.message, CabalEntry.requestedToast)
        XCTAssertEqual(model.standing, .requested(requestID: "r1"))
        XCTAssertEqual(model.membershipChanges, 0)
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths.dropFirst().first, "/v1/cabals/\(Self.cabalID)/access-requests")
    }

    func testAJoinRefusalKeepsTheButton() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request"))),
            Self.problem(409, "cabal_banned", "This cabal was banned."),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.enter()
        XCTAssertEqual(model.toast?.message, "This cabal was banned.")
        XCTAssertEqual(model.standing, .join)
    }

    func testCancellingRevokesTheRequest() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request", request: ("r1", "request")))),
            .json(.ok, #"{"id":"r1","direction":"request","status":"revoked"}"#),
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request"))),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.cancelRequest()
        XCTAssertEqual(model.standing, .join)
        let sent = await transport.sent
        XCTAssertEqual(sent[1].method, .delete)
        XCTAssertEqual(sent[1].path, "/v1/cabals/\(Self.cabalID)/access-requests/r1")
    }

    func testACancelThatFailsSaysWhy() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request", request: ("r1", "request")))),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.cancelRequest()
        XCTAssertEqual(model.toast?.message, "You're offline. Try again.")
        XCTAssertEqual(model.standing, .requested(requestID: "r1"))
    }

    func testEachAccessHintReloads() async throws {
        let transport = StubTransport(.json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request"))))
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        let observing = Task { await model.observe() }
        defer { observing.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 3 }
        await hints.send(.changed(.user("me"), what: "cabal_access", id: "x"))
        let first = await waitUntil { await transport.sent.count == 1 }
        XCTAssertTrue(first)
        await hints.send(.changed(.cabal(Self.cabalID), what: "access_requests", id: "x"))
        let second = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(second)
        await hints.send(.changed(.cabal("other"), what: "access_requests", id: "x"))
        for _ in 0..<50 { await Task.yield() }
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testThePreviewApprovesFromTheFixtures() async {
        let model = CabalAccessModel.preview(cabal: Self.cabal(role: "creator", mode: "request"))
        await model.load()
        guard case .pending(let pending) = model.standing else { return XCTFail("got \(model.standing)") }
        await model.decide(pending[0], approve: true)
        guard case .pending(let rest) = model.standing else { return XCTFail("got \(model.standing)") }
        XCTAssertEqual(rest.count, 1)
        let visitor = CabalAccessModel.preview(cabal: Self.cabal(role: nil, mode: "request"))
        await visitor.load()
        await visitor.enter()
        XCTAssertEqual(visitor.standing, .requested(requestID: "preview"))
        await visitor.cancelRequest()
        XCTAssertEqual(visitor.standing, .join)
    }

    private static let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"

    private func creatorReplies() throws -> [StubTransport.Reply] {
        [
            .json(.ok, try Self.encode(Self.cabal(role: "creator", mode: "request"))),
            .json(.ok, try Self.encode(requests)),
        ]
    }

    private func creatorModel(_ transport: StubTransport) async throws -> CabalAccessModel {
        let model = makeModel(transport)
        await model.load()
        return model
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> CabalAccessModel {
        CabalAccessModel(
            cabalID: Self.cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints
        )
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func cabal(role: String?, mode: String, request: (String, String)? = nil)
        -> Components.Schemas.Cabal
    {
        var cabal = Components.Schemas.Cabal.sample(role: role)
        cabal.rules.joinMode = mode
        cabal.myAccessRequest = request.map { .init(id: $0.0, direction: $0.1, status: "pending") }
        return cabal
    }

    private static func encode(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private static func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"\#(message)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8)
        )
    }
}
