import MonacoAPI
import MonacoCore
import XCTest

final class CabalVoterChoiceTests: XCTestCase {
    private let creatorID = "c-creator"

    func testEveryoneSendsAllAndNoVoterIDs() {
        let body = CabalVoterChoice.everyone.patch(creatorID: creatorID)

        XCTAssertEqual(body, .init(voterMode: "all"))
        XCTAssertNil(body.voterIds)
    }

    func testAListWithoutTheCreatorAddsThemFirst() {
        let body = CabalVoterChoice.list(["b-member"]).patch(creatorID: creatorID)

        XCTAssertEqual(body, .init(voterMode: "list", voterIds: [creatorID, "b-member"]))
    }

    func testAListPutsTheCreatorFirstAndSortsTheRest() {
        let body = CabalVoterChoice.list(["z-member", creatorID, "a-member", "m-member"]).patch(creatorID: creatorID)

        XCTAssertEqual(body.voterIds, [creatorID, "a-member", "m-member", "z-member"])
    }

    func testACabalWhereEveryoneVotesReadsAsEveryone() {
        XCTAssertEqual(CabalVoterChoice(.sampleWithMembers(role: "creator")), .everyone)
    }

    func testAListCabalReadsTheMembersWhoCanVote() {
        var cabal = Components.Schemas.Cabal.sampleWithMembers(role: "creator")
        cabal.rules.voterMode = "list"
        cabal.members[2].canVote = false

        XCTAssertEqual(CabalVoterChoice(cabal), .list([cabal.members[0].userId, cabal.members[1].userId]))
    }
}
