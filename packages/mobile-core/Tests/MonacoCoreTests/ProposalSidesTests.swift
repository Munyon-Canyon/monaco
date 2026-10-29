import MonacoAPI
import XCTest

@testable import MonacoCore

final class ProposalSidesTests: XCTestCase {
    private let base = Date(timeIntervalSince1970: 1_792_662_780)

    private func voter(_ name: String, _ ballot: String?, at offset: TimeInterval? = nil, voted: Bool? = nil)
        -> ProposalVoter
    {
        ProposalVoter(
            id: name.lowercased(), ballot: ballot, voted: voted, castAt: offset.map { base.addingTimeInterval($0) },
            displayName: name)
    }

    private func make(_ voters: [ProposalVoter], visible: Bool = true, viewer: String = "you") -> ProposalSides {
        ProposalSides.make(voters: voters, members: [], viewerID: viewer, sidesVisible: visible)
    }

    func testFiftyVotersGroupByBallotAndTheRowsListYesThenNoThenNotVoted() {
        let voters = (0..<50).map { index -> ProposalVoter in
            let ballot: String? =
                switch index % 3 {
                case 0: "yes"
                case 1: "no"
                default: nil
                }
            return ProposalVoter(id: "u\(index)", ballot: ballot, displayName: "Member")
        }
        let sides = make(voters)
        XCTAssertEqual(sides.yes.count + sides.no.count + sides.notVoted.count, 50)
        XCTAssertEqual(sides.yes.count, 17)
        XCTAssertEqual(sides.notVoted.count, 16)
        XCTAssertEqual(sides.rows.count, 50)
        XCTAssertTrue(sides.rows.prefix(17).allSatisfy { $0.side == .yes })
        XCTAssertTrue(sides.rows.dropFirst(17).prefix(17).allSatisfy { $0.side == .no })
        XCTAssertTrue(sides.rows.suffix(16).allSatisfy { $0.side == .notVoted })
    }

    func testVisibleSidesListEachBallotByCastTimeThenName() {
        let sides = make([
            voter("Sam", "yes", at: 30), voter("Mia", "yes", at: 10), voter("Kai", "no", at: 20),
            voter("Priya", nil), voter("Jordan", nil),
        ])
        XCTAssertEqual(sides.yes.map(\.name), ["Mia", "Sam"])
        XCTAssertEqual(sides.no.map(\.name), ["Kai"])
        XCTAssertEqual(sides.notVoted.map(\.name), ["Jordan", "Priya"])
        XCTAssertTrue(sides.votedHidden.isEmpty)
        XCTAssertEqual(sides.strip, "2 yes · 1 no · 2 not voted")
    }

    func testEqualCastTimesFallBackToName() {
        let sides = make([voter("Sam", "yes", at: 10), voter("Mia", "yes", at: 10)])
        XCTAssertEqual(sides.yes.map(\.name), ["Mia", "Sam"])
    }

    func testPrivateSidesKeepOnlyTheViewersBallot() {
        let sides = make(
            [
                voter("Mia", "yes"), voter("Kai", "no"), voter("You", "yes", at: 5),
                voter("Jordan", nil),
            ], visible: false)
        XCTAssertEqual(sides.yes.map(\.name), ["You"])
        XCTAssertTrue(sides.no.isEmpty)
        XCTAssertEqual(sides.votedHidden.map(\.name), ["Kai", "Mia"])
        XCTAssertEqual(sides.notVoted.map(\.name), ["Jordan"])
        XCTAssertEqual(sides.strip, "3 voted · 1 not voted")
        XCTAssertEqual(
            sides.rows.map(\.side), [.yes, .voted, .voted, .notVoted])
    }

    func testARedactedVoterWhoVotedIsHiddenNotNotVoted() {
        let sides = make([voter("Mia", nil, voted: true)], visible: false)
        XCTAssertEqual(sides.votedHidden.count, 1)
        XCTAssertTrue(sides.notVoted.isEmpty)
    }

    func testTheViewerComesLastAndReadsYou() {
        let sides = make([voter("You", "yes", at: 1), voter("Mia", "yes", at: 2), voter("Sam", "yes", at: 3)])
        XCTAssertEqual(sides.yes.map(\.name), ["Mia", "Sam", "You"])
        XCTAssertTrue(sides.yes.last?.isYou == true)
        XCTAssertEqual(sides.line(for: .yes), "Mia, Sam, you")
    }

    func testExactlyFiveNamesAreAllShown() {
        let sides = make([
            voter("Mia", "yes", at: 1), voter("Christelle", "yes", at: 2), voter("Logan", "yes", at: 3),
            voter("Sam", "yes", at: 4), voter("You", "yes", at: 5),
        ])
        XCTAssertEqual(sides.line(for: .yes), "Mia, Christelle, Logan, Sam, you")
    }

    func testSixNamesShowFourAndCountTheViewerInTheRest() {
        let sides = make([
            voter("Mia", "yes", at: 1), voter("Christelle", "yes", at: 2), voter("Logan", "yes", at: 3),
            voter("Sam", "yes", at: 4), voter("Kai", "yes", at: 5), voter("You", "yes", at: 6),
        ])
        XCTAssertEqual(sides.line(for: .yes), "Mia, Christelle, Logan, Sam and 2 more")
    }

    func testASingleNonVoter() {
        XCTAssertEqual(make([voter("Jordan", nil)]).line(for: .notVoted), "Jordan did not vote")
    }

    func testTwoNonVoters() {
        XCTAssertEqual(
            make([voter("Priya", nil), voter("Jordan", nil)]).line(for: .notVoted), "Jordan and Priya did not vote")
    }

    func testSixNonVoters() {
        let names = ["Jordan", "Priya", "Kai", "Sam", "Lee", "Max"]
        let sides = make(names.map { voter($0, nil) })
        XCTAssertEqual(sides.line(for: .notVoted), "Jordan, Kai, Lee, Max and 2 more did not vote")
    }

    func testNoNonVoters() {
        let sides = make([voter("Mia", "yes", at: 1)])
        XCTAssertTrue(sides.notVoted.isEmpty)
        XCTAssertEqual(sides.line(for: .notVoted), "")
    }

    func testSpokenLineJoinsTheLastNameWithAnd() {
        let sides = make([voter("Mia", "yes", at: 1), voter("Sam", "yes", at: 2), voter("You", "yes", at: 3)])
        XCTAssertEqual(sides.spokenLine(for: .yes), "Yes: Mia, Sam and you")
    }

    func testAMemberFillsInAnEmptyName() throws {
        let member = try XCTUnwrap(
            Components.Schemas.Cabal.sampleWithMembers(role: "member").members.map(ProposalMember.init).first)
        let sides = ProposalSides.make(
            voters: [ProposalVoter(id: member.id, ballot: "yes")], members: [member], viewerID: "x", sidesVisible: true)
        XCTAssertEqual(sides.yes.first?.name, member.name)
    }
}
