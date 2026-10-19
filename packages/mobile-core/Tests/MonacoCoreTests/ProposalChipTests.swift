import MonacoAPI
import XCTest

@testable import MonacoCore

final class ProposalChipTests: XCTestCase {
    func testEveryStatusForBuyAndSell() {
        let expected: [ProposalStatus: (String?, String?)] = [
            .open: (nil, nil), .passed: ("Buying", "Selling"), .executed: ("Bought", "Sold"),
            .failed: ("Didn't pass", "Didn't pass"), .expired: ("Expired", "Expired"),
            .withdrawn: ("Withdrawn", "Withdrawn"), .voided: ("Voided by Monaco", "Voided by Monaco"),
            .executionBlocked: ("Couldn't buy", "Couldn't sell"),
        ]
        for status in ProposalStatus.allCases {
            XCTAssertEqual(ProposalChip.label(status: status, isSell: false), expected[status]?.0)
            XCTAssertEqual(ProposalChip.label(status: status, isSell: true), expected[status]?.1)
        }
    }

    func testFailedSwapOverridesPassed() {
        XCTAssertEqual(ProposalChip.label(status: .passed, isSell: false, swapFailed: true), "Couldn't buy")
        XCTAssertEqual(ProposalChip.label(status: .passed, isSell: true, swapFailed: true), "Couldn't sell")
    }

    private typealias Badge = ProposalBadge

    private func summary(
        status: ProposalStatus = .open, kind: Components.Schemas.ProposalKind = .buy, ballot: String? = nil,
        canVote: Bool = true, expiresIn: TimeInterval = 3_600, swapStatus: String? = nil
    ) -> ProposalSummary {
        var proposal = Components.Schemas.Proposal.sample(
            status: Components.Schemas.ProposalStatus(rawValue: status.rawValue) ?? .open, kind: kind)
        proposal.expiresAt = Date().addingTimeInterval(expiresIn)
        proposal.myBallot = ballot.flatMap(Components.Schemas.Proposal.MyBallotPayload.init(rawValue:))
        let swap = swapStatus.flatMap(Components.Schemas.ProposalDetail.SwapPayload.StatusPayload.init(rawValue:))
        return ProposalSummary(
            proposal, canVote: canVote,
            swap: swap.map {
                ProposalSwap(
                    Components.Schemas.ProposalDetail.SwapPayload(
                        swapId: "swap-1", status: $0, failureCode: nil, failureMessage: nil, txSignature: nil,
                        retryable: false, tokenDecimals: 8))
            })
    }

    func testStateFollowsTheTableForEveryStatusKindAndBallot() {
        for kind in [Components.Schemas.ProposalKind.buy, .sell] {
            let now = Date()
            XCTAssertEqual(summary(kind: kind).state(now: now), .open)
            XCTAssertEqual(summary(kind: kind, ballot: "yes").state(now: now), .voted(.yes))
            XCTAssertEqual(summary(kind: kind, ballot: "no").state(now: now), .voted(.no))
            XCTAssertEqual(summary(kind: kind, canVote: false).state(now: now), .viewOnly)
            XCTAssertEqual(summary(status: .passed, kind: kind).state(now: now), .approved)
            XCTAssertEqual(summary(status: .executed, kind: kind).state(now: now), .executed)
            XCTAssertEqual(summary(status: .expired, kind: kind).state(now: now), .expired)
            XCTAssertEqual(summary(status: .executionBlocked, kind: kind).state(now: now), .blocked)
            XCTAssertEqual(summary(status: .passed, kind: kind, swapStatus: "failed").state(now: now), .blocked)
            XCTAssertEqual(summary(status: .failed, kind: kind).state(now: now), .rejected)
            XCTAssertEqual(summary(status: .withdrawn, kind: kind).state(now: now), .withdrawn)
            XCTAssertEqual(summary(status: .voided, kind: kind).state(now: now), .voided)
        }
    }

    func testAPassedProposalWithAPendingSwapIsStillApproved() {
        XCTAssertEqual(summary(status: .passed, swapStatus: "submitted").state(now: Date()), .approved)
    }

    func testAnOpenProposalPastItsExpiryCannotBeVotedOn() {
        let lapsed = summary(expiresIn: -60)
        XCTAssertEqual(lapsed.state(now: Date()), .viewOnly)
        XCTAssertEqual(summary(ballot: "no", expiresIn: -60).state(now: Date()), .voted(.no))
    }

    func testOnlyTheOpenStatesAreOpen() {
        let open: [ProposalState] = [.open, .voted(.yes), .voted(.no), .viewOnly]
        let closed: [ProposalState] = [.approved, .executed, .expired, .blocked, .rejected, .withdrawn, .voided]
        XCTAssertTrue(open.allSatisfy(\.isOpen))
        XCTAssertFalse(closed.contains(where: \.isOpen))
    }

    func testHeroBadgeForEveryState() {
        let table: [(ProposalState, String, ProposalBadge.Tone)] = [
            (.open, "Voting open", .brand), (.voted(.yes), "You voted Yes", .brand),
            (.voted(.no), "You voted No", .brand), (.viewOnly, "View only", .neutral),
            (.approved, "Approved · Executing", .warn), (.executed, "Executed", .brand),
            (.expired, "Expired", .neutral), (.blocked, "Not executed", .negative),
            (.rejected, "Not approved", .neutral), (.withdrawn, "Withdrawn", .neutral),
            (.voided, "Voided by Monaco", .neutral),
        ]
        for (state, label, tone) in table {
            XCTAssertEqual(ProposalBadge.hero(state).label, label, "\(state)")
            XCTAssertEqual(ProposalBadge.hero(state).tone, tone, "\(state)")
        }
    }

    func testCardBadgeForEveryState() {
        let table: [(ProposalState, String, ProposalBadge.Tone)] = [
            (.open, "Needs your vote", .brand), (.voted(.yes), "You voted Yes", .brand),
            (.voted(.no), "You voted No", .brand), (.viewOnly, "Voting open", .brand),
            (.approved, "Execution pending", .warn), (.executed, "Executed", .brand),
            (.expired, "Expired", .neutral), (.blocked, "Trade not completed", .negative),
            (.rejected, "Not approved", .neutral), (.withdrawn, "Withdrawn", .neutral),
            (.voided, "Voided by Monaco", .neutral),
        ]
        for (state, label, tone) in table {
            XCTAssertEqual(ProposalBadge.card(state).label, label, "\(state)")
            XCTAssertEqual(ProposalBadge.card(state).tone, tone, "\(state)")
        }
    }

    func testListRowBadgeForEveryState() {
        let table: [(ProposalState, String, ProposalBadge.Tone)] = [
            (.open, "Open", .brand), (.voted(.no), "Open", .brand), (.viewOnly, "Open", .brand),
            (.approved, "Pending", .warn), (.executed, "Executed", .brand), (.expired, "Expired", .negative),
            (.blocked, "Blocked", .negative), (.rejected, "Not approved", .neutral),
            (.withdrawn, "Withdrawn", .neutral), (.voided, "Voided", .neutral),
        ]
        for (state, label, tone) in table {
            XCTAssertEqual(ProposalBadge.listRow(state).label, label, "\(state)")
            XCTAssertEqual(ProposalBadge.listRow(state).tone, tone, "\(state)")
        }
    }

    func testBallotBadges() {
        XCTAssertEqual(ProposalBadge.ballot(.yes).label, "Yes")
        XCTAssertEqual(ProposalBadge.ballot(.yes).tone, .brand)
        XCTAssertEqual(ProposalBadge.ballot(.no).label, "No")
        XCTAssertEqual(ProposalBadge.ballot(.no).tone, .negative)
        XCTAssertEqual(ProposalBadge.ballot(nil).label, "Not voted")
        XCTAssertEqual(ProposalBadge.ballot(nil).tone, .neutral)
    }

    func testMapsEveryGeneratedStatus() {
        for status in Components.Schemas.ProposalStatus.allCases {
            XCTAssertEqual(ProposalStatus(status).rawValue, status.rawValue)
        }
    }
}
