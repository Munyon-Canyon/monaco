import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

private func voter(_ id: String, choice: Components.Schemas.ProposalVoteItem.ChoicePayload? = nil) -> ProposalVoterRow {
    ProposalVoterRow(
        Components.Schemas.ProposalVoteItem(
            userId: id, voted: choice != nil, handle: nil, displayName: "Voter \(id)", photoUrl: nil, choice: choice,
            castAt: choice == nil ? nil : Date(timeIntervalSince1970: 1_790_000_000)))
}

@MainActor
final class ProposalVotersPagerTests: XCTestCase {

    func testTwoPagesAppendWithoutRepeatingAUserIDAndKeepTheBallots() async {
        let pager = ProposalVotersPager { cursor in
            if cursor == nil { return ([voter("a", choice: .yes), voter("b")], "next") }
            return ([voter("c", choice: .no)], nil)
        }

        await pager.loadFirst()
        await pager.loadMore()

        XCTAssertEqual(pager.rows.map(\.id), ["a", "b", "c"])
        XCTAssertEqual(pager.rows.map(\.ballot), ["yes", nil, "no"])
        XCTAssertEqual(pager.rows.first?.name, "Voter a")
        XCTAssertEqual(pager.phase, .exhausted)
    }

    func testTotalsComeFromTheTallyNotFromThePagesLoadedSoFar() {
        let totals = ProposalVoterTotals(ProposalTally(yes: 40, no: 10, voters: 120, needed: 61))

        XCTAssertEqual(totals.notVoted, 70)
        XCTAssertEqual(totals.strip(sidesVisible: true), "40 yes · 10 no · 70 not voted")
        XCTAssertEqual(totals.strip(sidesVisible: false), "50 voted · 70 not voted")
    }

    func testARowBecomesAVoterWithItsCastTimeAndName() {
        let row = voter("a", choice: .yes)

        XCTAssertEqual(row.voter.id, "a")
        XCTAssertEqual(row.voter.ballot, "yes")
        XCTAssertEqual(row.voter.displayName, "Voter a")
        XCTAssertEqual(row.voter.castAt, Date(timeIntervalSince1970: 1_790_000_000))
    }
}
