import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalAccessVoterGrantTests: XCTestCase {
    private let requests = Components.Schemas.CabalAccessRequest.samples

    func testApprovingWithCanVoteOnPatchesTheNewMemberIntoTheVoters() async throws {
        var listCabal = Components.Schemas.Cabal.sampleWithMembers(role: "creator")
        listCabal.rules.joinMode = "request"
        listCabal.rules.voterMode = "list"
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(listCabal)),
            .json(.ok, try Self.encode(requests)),
            .json(.ok, #"{"id":"r","direction":"request","status":"approved"}"#),
            .json(.ok, try Self.encode(listCabal)),
            .json(.ok, try Self.encode(listCabal)),
            .json(.ok, try Self.encode(listCabal)),
            .json(.ok, try Self.encode([requests[1]])),
        ])
        let model = makeModel(transport)
        await model.load()
        XCTAssertTrue(model.picksVoters)
        guard case .pending(let pending) = model.standing else { return XCTFail("got \(model.standing)") }

        await model.decide(pending[0], approve: true, canVote: true)

        XCTAssertEqual(model.toast?.message, "Approved.")
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .get, .post, .get, .patch, .get, .get])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies[4])
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: Any])
        XCTAssertEqual(json["voter_mode"] as? String, "list")
        let ids = try XCTUnwrap(json["voter_ids"] as? [String])
        XCTAssertEqual(ids.first, listCabal.creator.userId)
        XCTAssertTrue(ids.contains(pending[0].userID))
    }

    func testApprovingWithCanVoteOffLeavesTheVotersAlone() async throws {
        var listCabal = Components.Schemas.Cabal.sampleWithMembers(role: "creator")
        listCabal.rules.joinMode = "request"
        listCabal.rules.voterMode = "list"
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(listCabal)),
            .json(.ok, try Self.encode(requests)),
            .json(.ok, #"{"id":"r","direction":"request","status":"approved"}"#),
            .json(.ok, try Self.encode(listCabal)),
            .json(.ok, try Self.encode([requests[1]])),
        ])
        let model = makeModel(transport)
        await model.load()
        guard case .pending(let pending) = model.standing else { return XCTFail("got \(model.standing)") }

        await model.decide(pending[0], approve: true, canVote: false)

        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .get, .post, .get, .get])
    }

    func testAFailedVoterGrantTellsTheCreatorWhereToFixIt() async throws {
        var listCabal = Components.Schemas.Cabal.sampleWithMembers(role: "creator")
        listCabal.rules.joinMode = "request"
        listCabal.rules.voterMode = "list"
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(listCabal)),
            .json(.ok, try Self.encode(requests)),
            .json(.ok, #"{"id":"r","direction":"request","status":"approved"}"#),
            .failure(URLError(.networkConnectionLost)),
            .json(.ok, try Self.encode(listCabal)),
            .json(.ok, try Self.encode([requests[1]])),
        ])
        let model = makeModel(transport)
        await model.load()
        guard case .pending(let pending) = model.standing else { return XCTFail("got \(model.standing)") }

        await model.decide(pending[0], approve: true, canVote: true)

        XCTAssertEqual(model.toast?.message, CabalAccessModel.approvedWithoutVoteToast)
        XCTAssertEqual(model.toast?.isSuccess, false)
    }

    private func makeModel(_ transport: StubTransport) -> CabalAccessModel {
        CabalAccessModel(
            cabalID: Components.Schemas.Cabal.sample(role: "creator").id,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintStream()
        )
    }

    private static func encode(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }
}
