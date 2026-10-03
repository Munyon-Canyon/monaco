import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalInvitesModelTests: XCTestCase {

    func testLoadShowsTheCabalItsMembersAndWhoInvited() async throws {
        let transport = StubTransport(.json(.ok, try Self.encode(Components.Schemas.CabalInvite.samples)))
        let model = makeModel(transport)
        await model.load()
        XCTAssertEqual(model.invites.map(\.cabalName), ["Friday Fund", "Long Haul"])
        XCTAssertEqual(model.invites.map(\.members), ["4 members", "1 member"])
        XCTAssertEqual(model.invites.map(\.invitedBy), ["@kaicenat invited you", "Mara invited you"])
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths, ["/v1/me/cabal-invites"])
    }

    func testAcceptApprovesTheInviteThenReloads() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode([Components.Schemas.CabalInvite.sample])),
            .json(.ok, #"{"id":"r","direction":"invite","status":"approved"}"#),
            .json(.ok, "[]"),
        ])
        let model = makeModel(transport)
        await model.load()
        let invite = try XCTUnwrap(model.invites.first)
        let accepted = await model.accept(invite)
        XCTAssertTrue(accepted)
        XCTAssertEqual(model.toast?.message, "You're in.")
        XCTAssertEqual(model.toast?.isSuccess, true)
        XCTAssertEqual(model.state, .loaded([]))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .post, .get])
        XCTAssertEqual(
            sent[1].path,
            "/v1/cabals/\(invite.cabalID)/access-requests/\(invite.id)/decision"
        )
        XCTAssertNotNil(sent[1].headerFields[try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies[1])
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["decision": "approve"])
    }

    func testDeclineDeniesAndRemovesTheRow() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Components.Schemas.CabalInvite.samples)),
            .json(.ok, #"{"id":"r","direction":"invite","status":"denied"}"#),
            .gate,
        ])
        let model = makeModel(transport)
        await model.load()
        let invite = try XCTUnwrap(model.invites.first)
        let declining = Task { await model.decline(invite) }
        let reloading = await waitUntil { await transport.sent.count == 3 }
        XCTAssertTrue(reloading)
        XCTAssertEqual(model.invites.map(\.cabalName), ["Long Haul"])
        XCTAssertEqual(model.toast?.message, "Invite declined.")
        await transport.releaseGate(.json(.ok, try Self.encode([Components.Schemas.CabalInvite.sampleSolo])))
        await declining.value
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies[1])
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["decision": "deny"])
    }

    func testAnExpiredInviteLeavesTheListWithTheServerMessage() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Components.Schemas.CabalInvite.samples)),
            Self.problem(409, "invite_expired", "This invite has expired."),
        ])
        let model = makeModel(transport)
        await model.load()
        let invite = try XCTUnwrap(model.invites.first)
        let accepted = await model.accept(invite)
        XCTAssertFalse(accepted)
        XCTAssertEqual(model.invites.map(\.cabalName), ["Long Haul"])
        XCTAssertEqual(model.toast?.message, "This invite has expired.")
        XCTAssertEqual(model.toast?.isSuccess, false)
    }

    func testAnInviteNoLongerPendingLeavesTheList() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Components.Schemas.CabalInvite.samples)),
            Self.problem(409, "access_request_not_pending", "This invite was withdrawn."),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.decline(try XCTUnwrap(model.invites.last))
        XCTAssertEqual(model.invites.map(\.cabalName), ["Friday Fund"])
        XCTAssertEqual(model.toast?.message, "This invite was withdrawn.")
    }

    func testAnyOtherRefusalKeepsTheRow() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Components.Schemas.CabalInvite.samples)),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        let accepted = await model.accept(try XCTUnwrap(model.invites.first))
        XCTAssertFalse(accepted)
        XCTAssertEqual(model.invites.count, 2)
        XCTAssertEqual(model.toast?.message, "You're offline. Try again.")
    }

    func testTheInvitesHintReloadsAndOtherUserHintsDoNot() async throws {
        let transport = StubTransport(.json(.ok, "[]"))
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        let observing = Task { await model.observe() }
        defer { observing.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)
        await hints.send(.changed(.user("me"), what: "cabals", id: "x"))
        await hints.send(.changed(.user("me"), what: "cabal_invites", id: "x"))
        let reloaded = await waitUntil { await transport.sent.count == 1 }
        XCTAssertTrue(reloaded)
        for _ in 0..<50 { await Task.yield() }
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testAHiddenTabWaitsToReloadUntilItShows() async throws {
        let transport = StubTransport(.json(.ok, "[]"))
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        let observing = Task { await model.observe() }
        defer { observing.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 1 }
        model.setVisible(false)
        await hints.send(.changed(.user("me"), what: "cabal_invites", id: "x"))
        for _ in 0..<50 { await Task.yield() }
        let whileHidden = await transport.sent.count
        XCTAssertEqual(whileHidden, 0)
        model.setVisible(true)
        let reloaded = await waitUntil { await transport.sent.count == 1 }
        XCTAssertTrue(reloaded)
    }

    func testThePreviewAnswersFromTheFixtures() async throws {
        let model = CabalInvitesModel.preview()
        await model.load()
        XCTAssertEqual(model.invites.count, 2)
        let accepted = await model.accept(try XCTUnwrap(model.invites.first))
        XCTAssertTrue(accepted)
        XCTAssertEqual(model.invites.map(\.cabalName), ["Long Haul"])
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> CabalInvitesModel {
        CabalInvitesModel(
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

    private static func encode(_ invites: [Components.Schemas.CabalInvite]) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(invites), as: UTF8.self)
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
