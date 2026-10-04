import MonacoCore
import XCTest

final class ProposalCardCopyTests: XCTestCase {
    func testClosesInHoursAndMinutes() {
        let now = Date(timeIntervalSince1970: 0)
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(23 * 3600), now: now), "Closes in 23h")
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(59 * 60), now: now), "Closes in 59m")
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(60), now: now), "Closes in 1m")
    }

    func testVoteCopy() {
        XCTAssertEqual(ProposalCardCopy.tracker(voted: 1, voters: 3, needed: 2), "1 of 3 voted · 2 yes to pass")
        XCTAssertEqual(ProposalCardCopy.voter("Priya", choice: "yes"), "Priya voted yes")
        XCTAssertEqual(ProposalCardCopy.voter("Priya", choice: "no"), "Priya voted no")
        XCTAssertEqual(ProposalCardCopy.voter("Priya", choice: nil), "Priya hasn't voted")
        XCTAssertEqual(ProposalCardCopy.viewerVote("yes"), "✓ You voted yes")
        XCTAssertEqual(ProposalCardCopy.viewerVote("no"), "✓ You voted no")
    }
}
