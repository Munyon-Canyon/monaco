import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class DeleteAccountModelTests: XCTestCase {
    func testLoadReadsTheBalanceAndTheCabalsTogether() async throws {
        let transport = RoutedTransport([
            "getMyBalance": [.json(.ok, Self.balance(available: "200150000"))],
            "getMyCabals": [.json(.ok, Self.cabals(["Weekend pot", "Work pot"]))],
        ])
        let model = makeModel(transport)

        await model.load()

        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(Set(paths), ["/v1/me/balance", "/v1/me/cabals"])
        let checklist = try XCTUnwrap(model.checklist)
        XCTAssertEqual(checklist.balance.availableMicros, 200_150_000)
        XCTAssertEqual(checklist.cabals.map(\.name), ["Weekend pot", "Work pot"])
        XCTAssertFalse(checklist.isCashedOut)
        XCTAssertFalse(checklist.isWithdrawn)
    }

    func testAnEmptyAccountHasBothStepsDone() async throws {
        let transport = RoutedTransport([
            "getMyBalance": [.json(.ok, Self.balance(available: "0"))], "getMyCabals": [.json(.ok, "[]")],
        ])
        let model = makeModel(transport)

        await model.load()

        let checklist = try XCTUnwrap(model.checklist)
        XCTAssertTrue(checklist.isCashedOut)
        XCTAssertTrue(checklist.isWithdrawn)
    }

    func testMoneyStillMovingKeepsTheWithdrawStepOpen() async throws {
        let transport = RoutedTransport([
            "getMyBalance": [.json(.ok, Self.balance(available: "0", inFlight: "5000000"))],
            "getMyCabals": [.json(.ok, "[]")],
        ])
        let model = makeModel(transport)

        await model.load()

        XCTAssertEqual(model.checklist?.isWithdrawn, false)
    }

    func testAFirstLoadThatFailsShowsTheFailure() async {
        let offline = StubTransport.Reply.failure(URLError(.notConnectedToInternet))
        let model = makeModel(RoutedTransport(["getMyBalance": [offline], "getMyCabals": [offline]]))

        await model.load()

        guard case .failed(let error) = model.state else {
            XCTFail("expected a failure, got \(model.state)")
            return
        }
        XCTAssertEqual(ToastCopy.message(for: error), "You're offline. Try again.")
    }

    func testAFailedReloadKeepsTheChecklistAndToasts() async throws {
        let offline = StubTransport.Reply.failure(URLError(.notConnectedToInternet))
        let transport = RoutedTransport([
            "getMyBalance": [.json(.ok, Self.balance(available: "0")), offline],
            "getMyCabals": [.json(.ok, "[]"), offline],
        ])
        let model = makeModel(transport)

        await model.load()
        await model.load()

        XCTAssertNotNil(model.checklist)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.toastMessage, "You're offline. Try again.")
    }

    func testARetryAfterALostResponseReusesTheIdempotencyKey() async throws {
        let transport = RoutedTransport([
            "deleteMe": [
                .failure(URLError(.networkConnectionLost)), .response(HTTPResponse(status: .noContent), Data()),
            ]
        ])
        let model = makeModel(transport)

        await model.delete()
        XCTAssertFalse(model.isDeleted)
        XCTAssertEqual(model.failureTick, 1)
        await model.delete()

        let keyName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.delete, .delete])
        XCTAssertEqual(sent.map(\.path), ["/v1/me", "/v1/me"])
        let keys = sent.map { $0.headerFields[keyName] }
        XCTAssertNotNil(keys[0])
        XCTAssertEqual(keys[0], keys[1])
        XCTAssertTrue(model.isDeleted)
        XCTAssertFalse(model.isDeleting)
    }

    func testABalanceRefusalHighlightsTheWithdrawStep() async throws {
        let transport = RoutedTransport([
            "getMyBalance": [.json(.ok, Self.balance(available: "200150000"))], "getMyCabals": [.json(.ok, "[]")],
            "deleteMe": [Self.problem("account_has_balance")],
        ])
        let model = makeModel(transport)
        await model.load()

        await model.delete()

        XCTAssertEqual(model.blocker, .withdrawFirst)
        XCTAssertEqual(model.highlighted, .withdrawFirst)
        XCTAssertEqual(model.toastMessage, "Withdraw your balance first.")
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertFalse(model.isDeleted)
    }

    func testTheHighlightClearsOnceTheStepIsDone() async throws {
        let transport = RoutedTransport([
            "getMyBalance": [
                .json(.ok, Self.balance(available: "200150000")), .json(.ok, Self.balance(available: "0")),
            ],
            "getMyCabals": [.json(.ok, "[]"), .json(.ok, "[]")],
            "deleteMe": [Self.problem("account_has_balance")],
        ])
        let model = makeModel(transport)
        await model.load()
        await model.delete()

        await model.load()

        XCTAssertEqual(model.blocker, .withdrawFirst)
        XCTAssertNil(model.highlighted)
    }

    func testAnAlreadyDeletedAccountCountsAsDeleted() async {
        let model = makeModel(RoutedTransport(["deleteMe": [Self.problem("account_deleted", status: 403)]]))

        await model.delete()

        XCTAssertTrue(model.isDeleted)
        XCTAssertEqual(model.failureTick, 0)
    }

    private func makeModel(_ transport: RoutedTransport) -> DeleteAccountModel {
        DeleteAccountModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        )
    }

    private static func balance(available: String, inFlight: String = "0") -> String {
        #"{"available_micros":"\#(available)","on_chain_micros":"\#(available)","#
            + #""in_flight_micros":"\#(inFlight)","deposit_address":"wallet-1","as_of":"2026-10-04T12:00:00Z"}"#
    }

    private static func cabals(_ names: [String]) -> String {
        let rows = names.enumerated().map { index, name in
            #"{"id":"01890a5d-ac96-774b-bcce-b302099a80\#(60 + index)","name":"\#(name)","picture_url":null,"#
                + #""role":"member","can_vote":true,"member_count":2,"joined_at":"2026-10-02T15:00:00Z","#
                + #""pending_request_count":0}"#
        }
        return "[\(rows.joined(separator: ","))]"
    }

    private static func problem(_ code: String, status: Int = 409) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"Server says no.","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8))
    }
}

private actor RoutedTransport: ClientTransport {
    private var replies: [String: [StubTransport.Reply]]
    private(set) var sent: [HTTPRequest] = []

    init(_ replies: [String: [StubTransport.Reply]]) {
        self.replies = replies
    }

    func send(_ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID: String) async throws
        -> (HTTPResponse, HTTPBody?)
    {
        sent.append(request)
        guard let reply = replies[operationID]?.first else {
            throw URLError(.cannotConnectToHost)
        }
        replies[operationID]?.removeFirst()
        switch reply {
        case .response(let response, let data): return (response, HTTPBody(data))
        case .failure(let error): throw error
        case .hang, .gate: throw URLError(.cannotConnectToHost)
        }
    }
}
