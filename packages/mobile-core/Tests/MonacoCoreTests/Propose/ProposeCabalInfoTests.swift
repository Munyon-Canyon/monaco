import MonacoAPI
import MonacoCore
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
}
