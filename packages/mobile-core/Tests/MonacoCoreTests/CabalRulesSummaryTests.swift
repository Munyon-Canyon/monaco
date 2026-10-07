import MonacoAPI
import MonacoCore
import XCTest

final class CabalRulesSummaryTests: XCTestCase {
    private func summary(_ edit: (inout Components.Schemas.Cabal) -> Void) -> CabalRulesSummary {
        var cabal = Components.Schemas.Cabal.sampleWithMembers(role: "member")
        edit(&cabal)
        return CabalRulesSummary(cabal)
    }

    private func voting(_ ids: Set<String>) -> (inout Components.Schemas.Cabal) -> Void {
        { cabal in
            cabal.rules.voterMode = "list"
            for index in cabal.members.indices {
                cabal.members[index].canVote = ids.contains(cabal.members[index].userId)
            }
        }
    }

    private var cabalName: String { Components.Schemas.Cabal.sampleWithMembers(role: "member").name }

    private var ids: [String] { Components.Schemas.Cabal.sampleWithMembers(role: nil).members.map(\.userId) }

    func testTheRowsComeInOrderWithTheirTitles() {
        let rows = summary { _ in }.rows

        XCTAssertEqual(rows.map(\.title), ["Name", "Who can join", "Who votes", "To pass", "Votes stay open"])
        XCTAssertEqual(rows.map(\.value), [cabalName, "Anyone", "Every member", "Everyone agrees", "1 day"])
    }

    func testTheNameRowShowsTheCabalName() {
        XCTAssertEqual(summary { $0.name = "Lunch club" }.name.value, "Lunch club")
    }

    func testJoinModes() {
        XCTAssertEqual(summary { $0.rules.joinMode = "open" }.join.value, "Anyone")
        XCTAssertEqual(summary { $0.rules.joinMode = "request" }.join.value, "The creator approves")
    }

    func testThresholds() {
        XCTAssertEqual(summary { $0.rules.threshold = "majority" }.threshold.value, "Majority")
        XCTAssertEqual(summary { $0.rules.threshold = "unanimous" }.threshold.value, "Everyone agrees")
    }

    func testExpiries() {
        XCTAssertEqual(summary { $0.rules.proposalExpirySeconds = 3600 }.expiry.value, "1 hour")
        XCTAssertEqual(summary { $0.rules.proposalExpirySeconds = 86_400 }.expiry.value, "1 day")
        XCTAssertEqual(summary { $0.rules.proposalExpirySeconds = 604_800 }.expiry.value, "1 week")
    }

    func testOneVoterIsTheirName() {
        XCTAssertEqual(summary(voting([ids[0]])).voters.value, "Kai")
    }

    func testTwoVotersAreJoinedWithAnd() {
        XCTAssertEqual(summary(voting([ids[0], ids[2]])).voters.value, "Kai and Priya")
    }

    func testThreeVotersAreACommaListEndingInAnd() {
        XCTAssertEqual(summary(voting(Set(ids))).voters.value, "Kai, Jordan and Priya")
    }

    func testTheCreatorIsNamedFirstWhereverTheyAreInTheMemberList() {
        let value = summary { cabal in
            voting(Set(ids))(&cabal)
            cabal.members.append(cabal.members.removeFirst())
        }.voters.value

        XCTAssertEqual(value, "Kai, Jordan and Priya")
    }

    func testAMemberWithNoNameShowsTheirHandle() {
        let value = summary { cabal in
            voting([ids[0], ids[1]])(&cabal)
            cabal.members[1].displayName = ""
        }.voters.value

        XCTAssertEqual(value, "Kai and @jordan")
    }

    func testAMemberWithNeitherNameNorHandleShowsNothing() {
        var member = Components.Schemas.Cabal.sampleWithMembers(role: nil).members[1]
        member.displayName = ""
        member.handle = nil

        XCTAssertEqual(member.shownName, "")
    }

    func testAListWithNobodyVotingReadsEmpty() {
        XCTAssertEqual(summary(voting([])).voters.value, "")
    }

    func testUnknownServerValuesShowAsSent() {
        let rows = summary {
            $0.rules.joinMode = "invite_only"
            $0.rules.voterMode = "council"
            $0.rules.threshold = "two_thirds"
            $0.rules.proposalExpirySeconds = 7200
        }.rows

        XCTAssertEqual(rows.dropFirst().map(\.value), ["invite_only", "council", "two_thirds", "7200"])
    }
}
