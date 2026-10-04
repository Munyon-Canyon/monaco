import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class InviteMemberModelTests: XCTestCase {
    private static let cabalID = "00000000-0000-7000-8000-0000000000c1"
    private static let viewerID = "00000000-0000-7000-8000-0000000000a1"
    private static let otherID = "00000000-0000-7000-8000-0000000000b1"
    private nonisolated static let now = Date(timeIntervalSince1970: 1_790_000_000)

    func testLoadShowsEachPendingInviteWithItsInviterAndExpiry() async throws {
        let transport = StubTransport(
            .json(
                .ok,
                Self.invites([
                    Self.invite(id: "r1", handle: "qa_b", inviterID: Self.viewerID, inviterName: "Alfred", days: 7),
                    Self.invite(
                        id: "r2", handle: nil, name: "Cay", inviterID: Self.otherID, inviterName: "Kai", hours: 23),
                ])))
        let model = makeModel(transport)
        await model.load()
        guard case .loaded(let rows) = model.state else {
            return XCTFail("expected rows, got \(model.state)")
        }
        XCTAssertEqual(rows.map(\.invitee), ["@qa_b", "Cay"])
        XCTAssertEqual(rows.map(\.invitedBy), ["Invited by you", "Invited by Kai"])
        XCTAssertEqual(rows.map(\.expiry), ["Expires in 7 days", "Expires today"])
        XCTAssertEqual(rows.map(\.canRevoke), [true, false])
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths, ["/v1/cabals/\(Self.cabalID)/invites?status=pending"])
    }

    func testTheCreatorCanRevokeAnyInvite() async throws {
        let transport = StubTransport(
            .json(
                .ok,
                Self.invites([
                    Self.invite(id: "r2", handle: "cay", inviterID: Self.otherID, inviterName: "Kai", days: 3)
                ])))
        let model = makeModel(transport, standing: CabalInviteStanding(joinMode: .request, role: .creator))
        await model.load()
        guard case .loaded(let rows) = model.state else {
            return XCTFail("expected rows, got \(model.state)")
        }
        XCTAssertEqual(rows.map(\.canRevoke), [true])
    }

    func testSendStripsTheAtSignAndLowercasesThenReloads() async throws {
        let transport = StubTransport(scripted: [
            .json(.created, #"{"id":"r1","direction":"invite","status":"pending"}"#),
            .json(
                .ok,
                Self.invites([
                    Self.invite(id: "r1", handle: "qa_b", inviterID: Self.viewerID, inviterName: "A", days: 7)
                ])),
        ])
        let model = makeModel(transport)
        model.handle = "  @QA_b "
        await model.send()
        XCTAssertEqual(model.toast?.message, "Invite sent.")
        XCTAssertEqual(model.toast?.isSuccess, true)
        XCTAssertEqual(model.handle, "")
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.post, .get])
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(Self.cabalID)/invites")
        XCTAssertNotNil(sent.first?.headerFields[try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first ?? nil)
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["handle": "qa_b"])
        guard case .loaded(let rows) = model.state else {
            return XCTFail("expected rows after the reload, got \(model.state)")
        }
        XCTAssertEqual(rows.map(\.invitee), ["@qa_b"])
    }

    func testAnUnknownHandleKeepsWhatWasTypedAndSaysSo() async throws {
        let transport = StubTransport(Self.problem(404, "user_not_found", "No user has that handle."))
        let model = makeModel(transport)
        model.handle = "@nobody_zz"
        await model.send()
        XCTAssertEqual(model.toast?.message, "No one on Monaco has that handle.")
        XCTAssertEqual(model.toast?.isSuccess, false)
        XCTAssertEqual(model.handle, "@nobody_zz")
        let methods = await transport.sent.map(\.method)
        XCTAssertEqual(methods, [.post])
    }

    func testEachInviteRefusalHasItsOwnLine() {
        XCTAssertEqual(
            InviteMemberModel.sendFailure(Self.apiProblem(409, "already_member")), "They're already in this cabal.")
        XCTAssertEqual(
            InviteMemberModel.sendFailure(Self.apiProblem(409, "request_pending")),
            "They already have a pending invite or request."
        )
        XCTAssertEqual(
            InviteMemberModel.sendFailure(Self.apiProblem(404, "user_not_found")), "No one on Monaco has that handle.")
        XCTAssertEqual(
            InviteMemberModel.sendFailure(Self.apiProblem(403, "not_cabal_creator")), "server says not_cabal_creator")
        XCTAssertEqual(
            InviteMemberModel.sendFailure(Self.apiProblem(500, "brand_new_code")), "server says brand_new_code")
        XCTAssertEqual(
            InviteMemberModel.sendFailure(.transport(URLError(.notConnectedToInternet))), "You're offline. Try again.")
    }

    func testABlankHandleSendsNothing() async {
        let transport = StubTransport(.json(.created, "{}"))
        let model = makeModel(transport)
        model.handle = " @ "
        XCTAssertFalse(model.canSend)
        await model.send()
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testRevokeDeletesTheInviteThenReloads() async throws {
        let transport = StubTransport(scripted: [
            .json(
                .ok,
                Self.invites([
                    Self.invite(id: "r1", handle: "qa_b", inviterID: Self.viewerID, inviterName: "A", days: 7)
                ])),
            .json(.ok, #"{"id":"r1","direction":"invite","status":"revoked"}"#),
            .json(.ok, "[]"),
        ])
        let model = makeModel(transport)
        await model.load()
        guard case .loaded(let rows) = model.state, let invite = rows.first else {
            return XCTFail("expected a row, got \(model.state)")
        }
        await model.revoke(invite)
        XCTAssertEqual(model.toast?.message, "Invite revoked.")
        XCTAssertEqual(model.state, .loaded([]))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .delete, .get])
        XCTAssertEqual(sent[1].path, "/v1/cabals/\(Self.cabalID)/access-requests/r1")
        XCTAssertNotNil(sent[1].headerFields[try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))])
    }

    func testAMembersHintForThisCabalReloadsTheList() async throws {
        let transport = StubTransport(.json(.ok, "[]"))
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        let observing = Task { await model.observe() }
        defer { observing.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)
        await hints.send(.changed(.cabal("another-cabal"), what: "members", id: "x"))
        await hints.send(.changed(.cabal(Self.cabalID), what: "members", id: "x"))
        let reloaded = await waitUntil { await transport.sent.count == 1 }
        XCTAssertTrue(reloaded)
        await hints.send(.changed(.cabal(Self.cabalID), what: "updated", id: "x"))
        for _ in 0..<50 { await Task.yield() }
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testAMemberOfARequestCabalWhoIsNotTheCreatorCannotInvite() async throws {
        let transport = StubTransport(.json(.ok, Self.cabal(joinMode: "request", role: "member")))
        let access = CabalInviteAccessModel(cabalID: Self.cabalID, api: api(transport))
        await access.load()
        XCTAssertEqual(access.standing, CabalInviteStanding(joinMode: .request, role: .member))
        XCTAssertFalse(access.canInvite)
    }

    func testTheCreatorOfARequestCabalCanInvite() async throws {
        let transport = StubTransport(.json(.ok, Self.cabal(joinMode: "request", role: "creator")))
        let access = CabalInviteAccessModel(cabalID: Self.cabalID, api: api(transport))
        XCTAssertFalse(access.canInvite)
        await access.load()
        XCTAssertTrue(access.canInvite)
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths, ["/v1/cabals/\(Self.cabalID)"])
    }

    private func makeModel(
        _ transport: StubTransport,
        hints: FakeHintStream = FakeHintStream(),
        standing: CabalInviteStanding = CabalInviteStanding(joinMode: .open, role: .member)
    ) -> InviteMemberModel {
        InviteMemberModel(
            cabalID: Self.cabalID, standing: standing, viewerID: Self.viewerID,
            api: api(transport), hints: hints, now: { Self.now }
        )
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func person(id: String, handle: String?, name: String) -> String {
        let handle = handle.map { #""\#($0)""# } ?? "null"
        return #"{"user_id":"\#(id)","handle":\#(handle),"display_name":"\#(name)","photo_url":null}"#
    }

    private static func invite(
        id: String, handle: String?, name: String = "", inviterID: String, inviterName: String,
        days: Double = 0, hours: Double = 0
    ) -> String {
        let expires = now.addingTimeInterval(days * 86_400 + hours * 3_600)
        let stamp = ISO8601DateFormatter().string(from: expires)
        let user = person(id: "00000000-0000-7000-8000-0000000000d1", handle: handle, name: name)
        let inviter = person(id: inviterID, handle: "inviter", name: inviterName)
        return #"{"request_id":"\#(id)","user":\#(user),"invited_by":\#(inviter),"expires_at":"\#(stamp)"}"#
    }

    private static func invites(_ rows: [String]) -> String {
        "[" + rows.joined(separator: ",") + "]"
    }

    private static func cabal(joinMode: String, role: String) -> String {
        let creator = person(id: otherID, handle: "kai", name: "Kai")
        return """
            {"id":"\(cabalID)","name":"QA pot","picture_url":null,"status":"active",
            "rules":{"join_mode":"\(joinMode)","voter_mode":"all","threshold":"majority",
            "proposal_expiry_seconds":86400,"slippage_bps":100},
            "creator":\(creator),"member_count":2,"members":[],"me":{"role":"\(role)","can_vote":true},
            "my_access_request":null,"invite_code":"ABCD2345","treasury_address":"treasury-placeholder"}
            """
    }

    private static func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"\#(message)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8)
        )
    }

    private static func apiProblem(_ status: Int, _ code: String) -> APIError {
        .problem(
            ProblemError(
                status: status, code: .init(code), message: "server says \(code)",
                traceID: "00000000000000000000000000000000", retryable: false
            )
        )
    }
}
