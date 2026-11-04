import MonacoAPI
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

    func testThePeopleLineNamesUpToTwoAndCountsTheRest() {
        let rows: [(names: [String], count: Int32, line: String)] = [
            (["Christelle", "Logan"], 8, "Christelle, Logan and 6 others"),
            (["Christelle", "Logan"], 3, "Christelle and Logan"),
            (["Christelle"], 2, "Christelle"),
            ([], 8, "8 members"),
            ([], 1, "1 member"),
        ]
        for row in rows {
            XCTAssertEqual(CabalMenuActions.peopleLine(names: row.names, memberCount: row.count), row.line)
        }
    }

    private func portfolioCabal(
        preview: [Components.Schemas.PortfolioMemberPreview]?
    ) -> Components.Schemas.PortfolioCabal {
        Components.Schemas.PortfolioCabal(
            cabal: .sampleACCapital, valueMicros: 7_000_000_000, shareUnits: 7_000_000_000,
            netContributedMicros: 6_687_600_000, pnlMicros: 312_400_000, returnBps: 467, sliceBps: 6045,
            ownershipBps: 3500, role: .admin, canVote: true, potValueMicros: 20_000_000_000, memberCount: 8,
            openVoteCount: 1, memberPreview: preview)
    }

    private func member(_ id: String, _ name: String) -> Components.Schemas.PortfolioMemberPreview {
        Components.Schemas.PortfolioMemberPreview(userId: id, displayName: name, photoUrl: nil)
    }

    func testATileWithoutAMemberPreviewHasNoMemberNames() {
        XCTAssertEqual(CabalTile(portfolioCabal(preview: nil), unreadCount: 0).memberNames, [])
    }

    func testATileKeepsTheFirstNamesOfTheFirstTwoPreviewMembers() {
        let preview = [
            member("01890a5d-ac96-774b-bcce-b302099a8101", "Christelle Dupont"),
            member("01890a5d-ac96-774b-bcce-b302099a8102", "Logan"),
            member("01890a5d-ac96-774b-bcce-b302099a8103", "Mia Chen"),
        ]

        let tile = CabalTile(portfolioCabal(preview: preview), unreadCount: 0)

        XCTAssertEqual(tile.memberNames, ["Christelle", "Logan"])
    }
}
