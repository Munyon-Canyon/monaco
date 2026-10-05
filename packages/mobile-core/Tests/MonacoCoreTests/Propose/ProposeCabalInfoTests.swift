import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class ProposeCabalInfoTests: XCTestCase {
    func testEveryMemberVotingReadsAsAllMembers() {
        let info = ProposeCabalInfo(.sampleWithMembers(role: "creator"))

        XCTAssertEqual(info.name, "QA pot")
        XCTAssertEqual(info.voters, "All 3 members")
    }

    func testAListOfVotersNamesThem() {
        var cabal = Components.Schemas.Cabal.sampleWithMembers(role: "creator")
        cabal.members[2].canVote = false
        cabal.members[1].displayName = ""

        XCTAssertEqual(ProposeCabalInfo(cabal).voters, "Kai, jordan")
    }

    func testLoadReadsTheCabalAndSummarisesItsVoters() async throws {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let body = String(
            decoding: try encoder.encode(Components.Schemas.Cabal.sampleWithMembers(role: "member")), as: UTF8.self)
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"),
            transport: StubTransport(.json(.ok, body)))

        let info = try await ProposeCabalInfo.load(api: api, cabalID: "cabal-1")

        XCTAssertEqual(info, ProposeCabalInfo(name: "QA pot", voters: "All 3 members"))
    }
}
