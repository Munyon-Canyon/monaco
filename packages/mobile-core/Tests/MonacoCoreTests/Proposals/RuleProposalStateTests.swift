import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class RuleProposalStateTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_789_682_400)

    private func summary(
        _ status: Components.Schemas.ProposalStatus, canVote: Bool = true, ballot: String? = nil
    ) -> ProposalSummary {
        var proposal = Components.Schemas.Proposal.ruleSample(now: now)
        proposal.status = status
        proposal.canVote = canVote
        proposal.myBallot = ballot.flatMap { Components.Schemas.Proposal.MyBallotPayload(rawValue: $0) }
        return ProposalSummary(proposal)
    }

    func testAnOpenRuleProposalReadsAsVotingNotAsABuy() {
        let open = summary(.open)

        XCTAssertTrue(open.isRule)
        XCTAssertEqual(open.state(now: now), .open)
        XCTAssertEqual(ProposalBadge.hero(open.state(now: now)).label, "Voting open")
        XCTAssertEqual(open.ruleChange?.title, "Show who voted what")
        XCTAssertEqual(open.ruleChange?.changeLine, "Hidden \u{2192} Shown")
        let steps = ProposalStepper.make(
            status: .open, isSell: false, expiresAt: open.expiresAt, isRule: true
        ).steps.map(\.title)
        XCTAssertEqual(steps, ["Voting", "Applied"])
    }

    func testAPassedRuleProposalIsAppliedAndNeverBuyingOrExecuting() {
        let passed = summary(.passed)
        let state = passed.state(now: now)

        XCTAssertEqual(state, .applied)
        XCTAssertEqual(ProposalBadge.hero(state).label, "Applied")
        XCTAssertEqual(ProposalBadge.card(state).label, "Applied")
        XCTAssertEqual(ProposalBadge.listRow(state).label, "Applied")
        XCTAssertEqual(ProposalChip.label(status: .passed, isSell: false, isRule: true), "Applied")
        XCTAssertFalse(passed.isTradeInProgress)

        let stepper = ProposalStepper.make(status: .passed, isSell: false, expiresAt: passed.expiresAt, isRule: true)
        XCTAssertEqual(stepper.steps.map(\.title), ["Voting", "Applied"])
        XCTAssertEqual(stepper.steps.map(\.mark), [.done, .done])
        let spoken = stepper.accessibilityLabel
        XCTAssertFalse(spoken.contains("Buying"))
        XCTAssertFalse(spoken.contains("in progress"))

        let texts: [String] = [
            ProposalCardCopy.note(
                state, needed: 5, expiresAt: passed.expiresAt, statusMessage: nil, now: now, isRule: true),
            ProposalCardCopy.footer(state, tally: passed.tally, isRule: true).text,
            ProposalCardCopy.rowSubtitle(state, proposer: "Mia", statusMessage: nil),
            ProposalCardCopy.statusNotice(state, failureMessage: nil, statusMessage: nil)?.message ?? "",
        ]
        for text in texts {
            XCTAssertFalse(text.contains("Execut"), text)
            XCTAssertFalse(text.contains("trade"), text)
            XCTAssertFalse(text.contains("pending"), text)
        }
    }

    func testAnExecutedRuleProposalIsAlsoApplied() {
        XCTAssertEqual(summary(.executed).state(now: now), .applied)
    }

    func testAFailedRuleProposalLeavesTheRuleUnchanged() {
        let failed = summary(.failed)
        let state = failed.state(now: now)

        XCTAssertEqual(state, .rejected)
        XCTAssertEqual(ProposalBadge.hero(state).label, "Not approved")
        let stepper = ProposalStepper.make(status: .failed, isSell: false, expiresAt: failed.expiresAt, isRule: true)
        XCTAssertEqual(stepper.steps.first?.title, "Didn't pass")
        XCTAssertEqual(stepper.steps.first?.note, "Not enough yes votes. The rule is unchanged.")
        let footer = ProposalCardCopy.footer(state, tally: failed.tally, isRule: true)
        XCTAssertEqual(footer.text, "Voting closed \u{b7} Rule unchanged")
        XCTAssertEqual(
            ProposalRuleCopy.caption(status: .failed, tally: failed.tally), "needed 5 \u{b7} rule unchanged")
        XCTAssertNil(ProposalResultCopy.action(status: .failed, canPropose: true, isRule: true))
        XCTAssertEqual(ProposalResultCopy.action(status: .failed, canPropose: true), .proposeAgain)
    }

    func testAPassedRuleProposalDrawsNoTradeRowAndSharesTheResult() {
        let passed = summary(.passed)

        XCTAssertFalse(ProposalResultCopy.drawsTradeRow(status: .passed, isRule: true))
        XCTAssertTrue(ProposalResultCopy.drawsTradeRow(status: .passed))
        XCTAssertEqual(ProposalResultCopy.action(status: .passed, canPropose: true, isRule: true), .share)
        XCTAssertEqual(
            ProposalResultCopy.caption(
                status: .passed, tally: passed.tally, swap: nil, isSell: false, pendingUnits: nil, statusMessage: nil,
                isRule: true),
            "rule applied")
    }

    func testTheVoteTallyOfASettledRuleSaysTheRuleWasAppliedOrKept() {
        let tally = ProposalTally(yes: 5, no: 1, voters: 8, needed: 5)

        XCTAssertEqual(
            ProposalVoteCopy.settledFacts(
                status: .passed, tally: tally, voted: 6, voters: 8, sharesEstimate: "4.5", isRule: true),
            ["Passed 5\u{2013}1", "6 of 8 voted", "rule applied"])
        XCTAssertEqual(
            ProposalVoteCopy.settledFacts(
                status: .failed, tally: tally, voted: 6, voters: 8, sharesEstimate: nil, isRule: true),
            ["Failed 5\u{2013}1", "6 of 8 voted", "rule unchanged"])
        XCTAssertEqual(
            ProposalVoteCopy.settledFacts(status: .failed, tally: tally, voted: 6, voters: 8, sharesEstimate: nil),
            ["Failed 5\u{2013}1", "6 of 8 voted", "no trade"])
    }

    func testTheFirstVotePushSampleOfARuleProposalNamesTheRuleChange() {
        let tally = ProposalTally(yes: 5, no: 1, voters: 8, needed: 5)
        let sample = PushSample.afterVote(
            symbol: "", status: .passed, tally: tally, isSell: false, usdcMicros: nil, closesAt: now, swap: nil,
            pendingUnits: nil, isRule: true)

        XCTAssertEqual(sample.title, "Rule change passed 5\u{2013}1")
        XCTAssertEqual(sample.body, "The rule is applied.")
    }

    func testEveryRuleValueHasWordsNotRawJSON() throws {
        let cases: [(String, Components.Schemas.Proposal.RuleChangePayload)] = [
            ("Hidden \u{2192} Shown", .init(rule: .showSides, from: false, to: true, user: nil)),
            (
                "Everyone agrees \u{2192} Majority",
                .init(rule: .threshold, from: "unanimous", to: "majority", user: nil)
            ),
            ("1 day \u{2192} 1 week", .init(rule: .proposalExpirySeconds, from: 86_400, to: 604_800, user: nil)),
            (
                "No limit \u{2192} 20% of cash",
                .init(rule: .maxTradeBps, from: .init(nilLiteral: ()), to: 2_000, user: nil)
            ),
            ("Open \u{2192} Approval required", .init(rule: .joinMode, from: "open", to: "request", user: nil)),
        ]
        for (expected, payload) in cases {
            var proposal = Components.Schemas.Proposal.ruleSample(now: now)
            proposal.ruleChange = payload
            XCTAssertEqual(ProposalSummary(proposal).ruleChange?.changeLine, expected)
        }
    }

    func testAMemberRuleNamesTheMember() {
        var proposal = Components.Schemas.Proposal.ruleSample(now: now)
        proposal.ruleChange = .init(
            rule: .voter, from: false, to: true,
            user: .init(userId: "user-kai", handle: "kai", displayName: "Kai", photoUrl: nil))

        let change = ProposalSummary(proposal).ruleChange
        XCTAssertEqual(change?.target, "Kai")
        XCTAssertEqual(change?.detailLine, "Who can vote \u{b7} Kai: Can't vote \u{2192} Can vote")
    }

    func testABuyKeepsItsTradeStates() {
        var proposal = Components.Schemas.Proposal.sample(status: .passed)
        proposal.canVote = false
        let buy = ProposalSummary(proposal)

        XCTAssertFalse(buy.isRule)
        XCTAssertNil(buy.ruleChange)
        XCTAssertEqual(buy.state(now: now), .approved)
        XCTAssertTrue(buy.isTradeInProgress)
        XCTAssertEqual(ProposalChip.label(status: .passed, isSell: false), "Buying")
    }
}
