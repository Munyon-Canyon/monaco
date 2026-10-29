import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class CabalMemberCopyTests: XCTestCase {
    private let page = Components.Schemas.CabalMemberPage.sampleACCapital

    private func row(
        _ role: Components.Schemas.CabalMemberRow.RolePayload, canVote: Bool = true, name: String = "Kai",
        handle: String? = "kai", month: [Int32] = [7, 9, 11, 11]
    ) -> Components.Schemas.CabalMemberRow {
        .init(
            userId: "u1", handle: handle, displayName: name, photoUrl: nil, role: role, canVote: canVote,
            joinedAt: Date(timeIntervalSince1970: 1_790_000_000), ownershipBps: 3500,
            month: .init(right: month[0], resolved: month[1], voted: month[2], eligible: month[3]), crown: false)
    }

    func testTheRecordLineCountsRightCallsAndTurnout() {
        XCTAssertEqual(CabalMemberCopy.recordLine(page.items[0].month), "7 of 9 right \u{00B7} voted 11 of 11")
        XCTAssertEqual(CabalMemberCopy.recordLine(page.items[1].month), "12 of 13 right \u{00B7} voted 14 of 14")
    }

    func testWithNoResolvedCallsTheLineIsTurnoutOnly() {
        let month = row(.member, month: [0, 0, 3, 3]).month
        XCTAssertEqual(CabalMemberCopy.recordLine(month), "voted 3 of 3")
    }

    func testWithNothingOpenTheLineSaysNoVotes() {
        let month = row(.member, month: [0, 0, 0, 0]).month
        XCTAssertEqual(CabalMemberCopy.recordLine(month), "No votes this month")
    }

    func testTheTabLinePrefixesEachRole() {
        XCTAssertEqual(CabalMemberCopy.tabLine(row(.creator)), "Admin \u{00B7} 7 of 9 right \u{00B7} voted 11 of 11")
        XCTAssertEqual(CabalMemberCopy.tabLine(row(.admin)), "Admin \u{00B7} 7 of 9 right \u{00B7} voted 11 of 11")
        XCTAssertEqual(CabalMemberCopy.tabLine(row(.member)), "Voter \u{00B7} 7 of 9 right \u{00B7} voted 11 of 11")
        XCTAssertEqual(
            CabalMemberCopy.tabLine(row(.member, canVote: false)),
            "Member \u{00B7} 7 of 9 right \u{00B7} voted 11 of 11")
    }

    func testTheVoterLineIsTheRecordForVotersAndHoldsAShareOtherwise() {
        XCTAssertEqual(CabalMemberCopy.voterLine(row(.member)), "7 of 9 right \u{00B7} voted 11 of 11")
        XCTAssertEqual(CabalMemberCopy.voterLine(row(.admin)), "7 of 9 right \u{00B7} voted 11 of 11")
        XCTAssertEqual(CabalMemberCopy.voterLine(row(.member, canVote: false)), "Holds a share")
    }

    func testTheViewerIsMarkedYou() {
        XCTAssertEqual(CabalMemberCopy.name(row(.admin, name: "Andres"), viewerID: "u1"), "Andres \u{00B7} you")
        XCTAssertEqual(CabalMemberCopy.name(row(.admin, name: "Andres"), viewerID: "u2"), "Andres")
        XCTAssertEqual(CabalMemberCopy.name(row(.admin, name: "Andres"), viewerID: nil), "Andres")
    }

    func testANamelessMemberShowsTheHandleOrSomeone() {
        XCTAssertEqual(CabalMemberCopy.name(row(.member, name: ""), viewerID: nil), "@kai")
        XCTAssertEqual(CabalMemberCopy.name(row(.member, name: "", handle: nil), viewerID: nil), "Someone")
    }

    func testTheShareRoundsDownAndUnderOnePercentReadsLessThanOne() {
        XCTAssertEqual(CabalMemberCopy.share(basisPoints: 3500), "35%")
        XCTAssertEqual(CabalMemberCopy.share(basisPoints: 1299), "12%")
        XCTAssertEqual(CabalMemberCopy.share(basisPoints: 99), "<1%")
        XCTAssertEqual(CabalMemberCopy.share(basisPoints: 1), "<1%")
        XCTAssertEqual(CabalMemberCopy.share(basisPoints: 0), "0%")
        XCTAssertEqual(CabalMemberCopy.share(basisPoints: 10000), "100%")
    }

    func testTheRequestLineNamesTheLinkWhenThereIsOne() {
        let created = Date(timeIntervalSince1970: 1_790_000_000)
        let now = created.addingTimeInterval(2 * 3600 + 30)
        XCTAssertEqual(
            CabalMemberCopy.requestLine(via: "Jordan", createdAt: created, now: now),
            "Asked to join \u{00B7} Jordan's link \u{00B7} 2h")
        XCTAssertEqual(
            CabalMemberCopy.requestLine(via: nil, createdAt: created, now: now), "Asked to join \u{00B7} 2h")
    }

    func testSectionTitlesTakeTheirCountsFromTheTotals() {
        let totals = Components.Schemas.CabalMemberTotals(admins: 2, voters: 80, members: 38)
        XCTAssertEqual(CabalMemberCopy.sectionTitle(.admin, totals: totals), "Admins 2")
        XCTAssertEqual(CabalMemberCopy.sectionTitle(.voter, totals: totals), "Voters 80")
        XCTAssertEqual(CabalMemberCopy.sectionTitle(.member, totals: totals), "Members 38")
        XCTAssertEqual(CabalMemberCopy.voteTitle(totals: totals), "Vote (82)")
        XCTAssertEqual(CabalMemberCopy.dontVoteTitle(totals: totals), "Don't vote (38)")
    }

    func testTheProposeActionsFollowTheCurrentRole() {
        XCTAssertEqual(CabalMemberCopy.voterAction(.member), "Propose making a voter")
        XCTAssertEqual(CabalMemberCopy.voterAction(.voter), "Propose removing as voter")
        XCTAssertEqual(CabalMemberCopy.adminAction(.voter), "Propose making an admin")
        XCTAssertEqual(CabalMemberCopy.adminAction(.admin), "Propose removing as admin")
    }

    func testTheSmallStringsRead() {
        XCTAssertEqual(CabalMemberCopy.seeAll(8), "See all 8")
        XCTAssertEqual(CabalMemberCopy.inviteTo("AC Capital"), "Invite to AC Capital")
        XCTAssertEqual(CabalMemberCopy.joinedToast(name: "Leo", cabal: "AC Capital"), "Leo joined AC Capital.")
    }
}
