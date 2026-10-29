import XCTest

@testable import MonacoCore

final class CabalMenuActionsTests: XCTestCase {
    private func vote(
        _ id: String = "proposal-1", kind: String = "buy", rule: String? = nil, symbol: String? = "NVDAx"
    ) -> HomePendingVote {
        HomePendingVote(
            id: id, cabalID: "cabal-1", cabalName: "AC Capital", kind: kind, rule: rule, symbol: symbol,
            assetName: "NVIDIA", usdcMicros: 400_000_000, tally: ProposalTally(yes: 4, no: 1, voters: 8, needed: 5),
            replyCount: 6, expiresAt: Date(timeIntervalSince1970: 1_999_000_000))
    }

    private func titles(canVote: Bool = true, restricted: Bool = false, waiting: [HomePendingVote]) -> [String] {
        CabalMenuActions.actions(canVote: canVote, restricted: restricted, waiting: waiting).map(\.title)
    }

    func testAVoterWithAWaitingVoteSeesAllFiveWithTheVoteFirst() {
        let actions = CabalMenuActions.actions(canVote: true, restricted: false, waiting: [vote()])

        XCTAssertEqual(actions.map(\.title), ["Vote on Buy NVDA", "Fund", "Propose buy", "Invite", "Share"])
        XCTAssertEqual(actions.map(\.kind), [.vote(proposalID: "proposal-1"), .fund, .propose, .invite, .share])
    }

    func testTheFirstThreeAreOneGroupAndTheLastTwoAnother() {
        let actions = CabalMenuActions.actions(canVote: true, restricted: false, waiting: [vote()])

        XCTAssertEqual(actions.map(\.group), [.trade, .trade, .trade, .people, .people])
    }

    func testNoVoteWaitingLeavesOutTheVote() {
        XCTAssertEqual(titles(waiting: []), ["Fund", "Propose buy", "Invite", "Share"])
    }

    func testANonVoterSeesNoProposeBuy() {
        XCTAssertEqual(titles(canVote: false, waiting: [vote()]), ["Vote on Buy NVDA", "Fund", "Invite", "Share"])
    }

    func testANonAdminStillSeesInvite() {
        XCTAssertTrue(titles(canVote: false, waiting: []).contains("Invite"))
    }

    func testARestrictedAccountSeesNoFund() {
        XCTAssertEqual(
            titles(restricted: true, waiting: [vote()]), ["Vote on Buy NVDA", "Propose buy", "Invite", "Share"])
    }

    func testTwoWaitingVotesGiveOneActionThatOpensTheCabal() {
        let actions = CabalMenuActions.actions(
            canVote: true, restricted: false, waiting: [vote("a"), vote("b", symbol: "TSLAx")])

        XCTAssertEqual(actions.first?.title, "Vote on 2 proposals")
        XCTAssertEqual(actions.first?.kind, .openCabal)
        XCTAssertEqual(actions.count, 5)
    }

    func testAWaitingRuleProposalReadsRuleChange() {
        let rule = vote(kind: "rule", rule: "threshold", symbol: nil)

        XCTAssertEqual(titles(waiting: [rule]).first, "Vote on Rule change")
    }

    func testAWaitingSellReadsSell() {
        XCTAssertEqual(titles(waiting: [vote(kind: "sell")]).first, "Vote on Sell NVDA")
    }

    func testTheActionIDsAreDistinct() {
        let actions = CabalMenuActions.actions(canVote: true, restricted: false, waiting: [vote()])

        XCTAssertEqual(Set(actions.map(\.id)).count, actions.count)
    }

    func testTheFiguresLineIsValueThenReturn() {
        XCTAssertEqual(
            CabalMenuActions.figuresLine(valueMicros: 7_000_000_000, returnText: "+4.7%"), "$7,000 \u{00B7} +4.7%")
    }

    func testTheMembersLineUsesTheHeaderRoleWording() {
        XCTAssertEqual(
            CabalMenuActions.membersLine(memberCount: 8, role: "admin", canVote: true),
            "8 members \u{00B7} you are admin")
        XCTAssertEqual(
            CabalMenuActions.membersLine(memberCount: 6, role: "member", canVote: true), "6 members \u{00B7} you vote")
        XCTAssertEqual(
            CabalMenuActions.membersLine(memberCount: 1, role: "member", canVote: false),
            "1 member \u{00B7} you hold a share")
    }
}
