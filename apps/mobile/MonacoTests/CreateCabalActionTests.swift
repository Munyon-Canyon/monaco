import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct CreateCabalActionTests {
    @Test func justMeSendsOnePostWithAKeyAndNoVoterIdsOrSlippage() async throws {
        let transport = StubTransport(.json(.created, Self.created))
        let actions = LiveCabalsActionSource(auth: Self.auth, api: Self.api(transport))
        let input = try #require(CreateCabalForm(name: " QA pot ", joinMode: .request, voterMode: .justMe).input)

        let cabal = try await actions.createCabal(input, submission: IdempotentSubmission())

        #expect(cabal.name == "QA pot")
        let sent = await transport.sent
        #expect(sent.map(\.method) == [.post])
        #expect(sent.map(\.path) == ["/v1/cabals"])
        let key = try #require(HTTPField.Name(IdempotentSubmission.keyHeader))
        #expect(sent.first?.headerFields[key]?.isEmpty == false)
        let bodies = await transport.sentBodies
        let data = try #require(bodies.first ?? nil)
        let body = try #require(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        #expect(body["name"] as? String == "QA pot")
        #expect(body["join_mode"] as? String == "request")
        #expect(body["voter_mode"] as? String == "list")
        #expect(body["slippage_bps"] == nil)
        #expect(body["voter_ids"] == nil)
    }

    @Test func aRetryAfterALostResponseReusesTheKey() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.networkConnectionLost)),
            .json(.created, Self.created),
        ])
        let actions = LiveCabalsActionSource(auth: Self.auth, api: Self.api(transport))
        let input = try #require(CreateCabalForm(name: "QA pot").input)
        let submission = IdempotentSubmission()

        await #expect(throws: APIError.self) {
            _ = try await actions.createCabal(input, submission: submission)
        }
        _ = try await actions.createCabal(input, submission: submission)

        let key = try #require(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.map { $0.headerFields[key] }
        #expect(keys.count == 2)
        #expect(keys[0] != nil)
        #expect(keys[0] == keys[1])
    }

    private static let auth = PrivyAuthService.processInstance ?? PrivyAuthService()

    private static func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-a"), transport: transport)
    }

    private static let created = """
        {"id":"01890a5d-ac96-774b-bcce-b302099a8060","name":"QA pot","picture_url":null,"status":"active",\
        "rules":{"join_mode":"request","voter_mode":"list","threshold":"majority","proposal_expiry_seconds":86400,"slippage_bps":100},\
        "creator":{"user_id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"kai","display_name":"Kai","photo_url":null},\
        "member_count":1,"members":[],"me":{"role":"creator","can_vote":true},"my_access_request":null,\
        "invite_code":"ABCD2345","treasury_address":"treasury-1"}
        """
}
