import MonacoCore
import XCTest

final class ProposalCardCopyTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_000_000)

    func testClosesInDaysHoursAndMinutes() {
        XCTAssertEqual(closes(in: 3 * 86_400), "Closes in 3d")
        XCTAssertEqual(closes(in: 47 * 3600), "Closes in 47h")
        XCTAssertEqual(closes(in: 23 * 3600), "Closes in 23h")
        XCTAssertEqual(closes(in: 59 * 60), "Closes in 59m")
        XCTAssertEqual(closes(in: 42 * 60 + 30), "Closes in 42m")
        XCTAssertEqual(closes(in: 60), "Closes in 1m")
        XCTAssertEqual(closes(in: -5), "Closes in 1m")
    }

    func testAge() {
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(-20), now: now), "now")
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(-33 * 60), now: now), "33m")
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(-5 * 3600), now: now), "5h")
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(-2 * 86_400), now: now), "2d")
        XCTAssertEqual(
            ProposalCardCopy.proposedBy("Jordan", since: now.addingTimeInterval(-33 * 60), now: now),
            "Proposed by Jordan · 33m")
    }

    func testVoteCopy() {
        XCTAssertEqual(ProposalCardCopy.tracker(voted: 1, voters: 3, needed: 2), "1 of 3 voted · 2 yes to pass")
        XCTAssertEqual(ProposalCardCopy.voter("Priya", choice: .yes), "Priya voted yes")
        XCTAssertEqual(ProposalCardCopy.voter("Priya", choice: .no), "Priya voted no")
        XCTAssertEqual(ProposalCardCopy.voter("Priya", choice: nil), "Priya hasn't voted")
        XCTAssertEqual(ProposalCardCopy.viewerVote(.yes), "✓ You voted yes")
        XCTAssertEqual(ProposalCardCopy.viewerVote(.no), "✓ You voted no")
        XCTAssertEqual(ProposalCardCopy.reasonTitle(.buy), "Why buy")
        XCTAssertEqual(ProposalCardCopy.reasonTitle(.sell), "Why sell")
    }

    private func closes(in seconds: TimeInterval) -> String {
        ProposalCardCopy.closes(at: now.addingTimeInterval(seconds), now: now)
    }
}
