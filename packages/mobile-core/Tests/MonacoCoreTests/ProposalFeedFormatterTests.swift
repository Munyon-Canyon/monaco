import XCTest

@testable import MonacoCore

final class ProposalFeedFormatterTests: XCTestCase {
    private let now = ISO8601DateFormatter().date(from: "2026-09-18T12:00:00Z")!

    func testAmount_formatsMicrosAsDollars() {
        // Arrange
        let micros = "1250000000"

        // Act
        let label = ProposalAmountFormatter.dollars(fromMicros: micros)

        // Assert
        XCTAssertEqual(label, "$1,250.00")
    }

    func testAmount_unparseable_returnsRawValue() {
        // Arrange
        let raw = "not-a-number"

        // Act
        let label = ProposalAmountFormatter.dollars(fromMicros: raw)

        // Assert
        XCTAssertEqual(label, raw)
    }

    func testClosesLabel_hoursLeft() {
        // Act
        let label = ProposalTimeFormatter.closesLabel(expiresAt: "2026-09-18T17:20:00Z", now: now)

        // Assert
        XCTAssertEqual(label, "Closes in 5h")
    }

    func testClosesLabel_buckets() {
        XCTAssertEqual(ProposalTimeFormatter.closesLabel(expiresAt: "2026-09-19T11:59:00Z", now: now), "Closes in 23h")
        XCTAssertEqual(ProposalTimeFormatter.closesLabel(expiresAt: "2026-09-19T20:00:00Z", now: now), "Closes in 32h")
        XCTAssertEqual(ProposalTimeFormatter.closesLabel(expiresAt: "2026-09-21T13:00:00Z", now: now), "Closes in 3d")
        XCTAssertEqual(ProposalTimeFormatter.closesLabel(expiresAt: "2026-09-18T12:12:30Z", now: now), "Closes in 12m")
        XCTAssertEqual(ProposalTimeFormatter.closesLabel(expiresAt: "2026-09-18T12:00:20Z", now: now), "Closes in 1m")
    }

    func testClosesSoon_onlyInTheLastHour() {
        XCTAssertTrue(ProposalTimeFormatter.closesSoon(expiresAt: "2026-09-18T12:40:00Z", now: now))
        XCTAssertFalse(ProposalTimeFormatter.closesSoon(expiresAt: "2026-09-18T13:00:01Z", now: now))
        XCTAssertFalse(ProposalTimeFormatter.closesSoon(expiresAt: "2026-09-18T11:00:00Z", now: now))
        XCTAssertFalse(ProposalTimeFormatter.closesSoon(expiresAt: "garbage", now: now))
    }

    func testClosesLabel_pastExpiry_saysClosed() {
        // Act
        let label = ProposalTimeFormatter.closesLabel(expiresAt: "2026-09-18T11:00:00Z", now: now)

        // Assert
        XCTAssertEqual(label, "Voting closed")
    }

    func testClosesLabel_garbage_isNil() {
        // Act
        let label = ProposalTimeFormatter.closesLabel(expiresAt: "tomorrow", now: now)

        // Assert
        XCTAssertNil(label)
    }

    func testAgeLabel_compactBuckets() {
        // Act
        let labels = [
            ProposalTimeFormatter.ageLabel("2026-09-18T11:59:30Z", now: now),
            ProposalTimeFormatter.ageLabel("2026-09-18T11:48:00Z", now: now),
            ProposalTimeFormatter.ageLabel("2026-09-18T09:00:00Z", now: now),
            ProposalTimeFormatter.ageLabel("2026-09-16T12:00:00Z", now: now),
        ]

        // Assert
        XCTAssertEqual(labels, ["now", "12m", "3h", "2d"])
    }

    func testProposalFeedCopy_passesMainFlowCopyAudit() {
        // Act
        let clean = MainFlowCopyAudit.stringsAreClean(ProposalFeedCopy.auditedStrings)

        // Assert
        XCTAssertTrue(clean)
    }

    func testProposeFlowCopy_passesMainFlowCopyAudit() {
        // Assert: every string passes, and none leaks jargon the plan bans from primary copy.
        XCTAssertTrue(MainFlowCopyAudit.stringsAreClean(ProposeFlowCopy.auditedStrings))
        let jargon = ["treasury", "route", "quote", "usdc", "bps", "units", "stake", "http", "api", "!"]
        for string in ProposeFlowCopy.auditedStrings + ProposalFeedCopy.auditedStrings {
            for term in jargon {
                XCTAssertFalse(string.lowercased().contains(term), "\"\(string)\" contains \"\(term)\"")
            }
        }
    }

    func testProposeFlowCopy_auditsConnectInstructionStrings() {
        let audited = ProposeFlowCopy.auditedStrings
        XCTAssertTrue(audited.contains(ProposeFlowCopy.copyConnectInstructions))
        XCTAssertTrue(audited.contains(ProposeFlowCopy.connectCopied))
        XCTAssertTrue(audited.contains(ProposeFlowCopy.clawPumpSteps))
        XCTAssertNotEqual(ProposeFlowCopy.connectCopied, ProposeFlowCopy.keyCopied)
    }

    func testVoteCopy_plainYesNo() {
        XCTAssertEqual(ProposalFeedCopy.voteYes, "Yes")
        XCTAssertEqual(ProposalFeedCopy.voteNo, "No")
        XCTAssertEqual(ProposalFeedCopy.viewerVoted("yes"), "You voted yes")
        XCTAssertEqual(ProposalFeedCopy.viewerVoted("NO"), "You voted no")
    }

    func testShares_wholeAndFractional() {
        // Act / Assert
        XCTAssertEqual(ProposalShareFormatter.shares(fromAtomics: "300000000"), "3")
        XCTAssertEqual(ProposalShareFormatter.shares(fromAtomics: "12345678"), "0.12345678")
        XCTAssertEqual(ProposalShareFormatter.shares(fromAtomics: "abc"), "abc")
    }

    func testReasonLength_matchesBackendThesisRule() {
        XCTAssertEqual(ProposeFlowCopy.reasonMax, 500)
        XCTAssertEqual(ProposeFlowCopy.reasonLength("  Earnings beat.  \n"), 14)
        XCTAssertEqual(ProposeFlowCopy.reasonLength("   "), 0)
        // The backend counts UTF-8 bytes, so accented text uses more of the limit.
        XCTAssertEqual(ProposeFlowCopy.reasonLength("é"), 2)
        XCTAssertEqual(ProposeFlowCopy.reasonCounter(480), "480 of 500")
    }
}
