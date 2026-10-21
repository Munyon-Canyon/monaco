import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class CabalCopyTests: XCTestCase {
    func testMemberCopy() {
        XCTAssertEqual(CabalCopy.memberCount(1), "1 member")
        XCTAssertEqual(CabalCopy.memberCount(3), "3 members")
        let named = member(displayName: "Kai", handle: "kai", role: "creator")
        XCTAssertEqual(CabalCopy.memberName(named), "Kai")
        XCTAssertEqual(CabalCopy.memberName(member(displayName: " ", handle: "kai", role: "member")), "@kai")
        XCTAssertEqual(CabalCopy.memberName(member(displayName: "", handle: nil, role: "member")), "Member")
    }

    func testTheFourMoneyTermsAreDistinct() {
        XCTAssertEqual(CabalCopy.moneyTerms, ["Deposit", "Fund this cabal", "Cash out", "Withdraw"])
        XCTAssertEqual(Set(CabalCopy.moneyTerms).count, 4)
    }

    func testOnlyDepositingNamesTheExternalToCashMovement() {
        let cashToCabal = [
            CabalCopy.fundTerm, CabalCopy.fundAction(amount: "$250.00"),
            CabalCopy.fundActivityTitle(cabalName: "Sunday Investors"),
            CabalCopy.fundActivityTitle(cabalName: "a cabal"),
            CabalCopy.fundAmountCaption, CabalCopy.fundSettledTitle, CabalCopy.noShareNotice,
            CabalCopy.cashAfterFunding("$250.00"), FundProgress.funding, FundOutcome.settledTitle,
        ]
        for line in cashToCabal {
            XCTAssertFalse(line.localizedCaseInsensitiveContains("deposit"), line)
        }
        XCTAssertTrue(CabalCopy.depositToCash.hasPrefix(CabalCopy.depositTerm))
        XCTAssertTrue(CabalCopy.fundNeedsCash.localizedCaseInsensitiveContains("deposit to your cash"))
    }

    func testTheNewFundingCopyPassesTheMainFlowAudit() {
        XCTAssertTrue(
            MainFlowCopyAudit.stringsAreClean([
                CabalCopy.fundTerm, CabalCopy.fundAmountCaption, CabalCopy.fundSettledTitle, CabalCopy.fundFromCash,
                CabalCopy.fundToPot, CabalCopy.fundNeedsCash, CabalCopy.depositToCash, CabalCopy.noShareNotice,
                CabalCopy.cashAfterFunding("$1.00"), CabalCopy.fundActivityTitle(cabalName: "a cabal"),
            ]))
    }

    func testFundingNamesWhereTheCashComesFromAndWhereItGoes() {
        XCTAssertEqual(CabalCopy.fundFromLabel, "From")
        XCTAssertEqual(CabalCopy.fundToLabel, "To")
        XCTAssertEqual(CabalCopy.fundFromCash, "Your cash")
        XCTAssertEqual(CabalCopy.fundToPot, "Shared pot")
        XCTAssertEqual(CabalCopy.fundAction(amount: "$250.00"), "Fund $250.00")
        XCTAssertEqual(CabalCopy.cashAfterFunding("$250.00"), "Cash after funding $250.00")
    }

    func testOnlyTheCreatorSeesPendingRequests() {
        XCTAssertEqual(CabalCopy.requestBadge(myCabal(role: "creator", pending: 2)), 2)
        XCTAssertNil(CabalCopy.requestBadge(myCabal(role: "creator", pending: 0)))
        XCTAssertNil(CabalCopy.requestBadge(myCabal(role: "member", pending: 2)))
        XCTAssertEqual(CabalCopy.requestCount(1), "1 request to join")
        XCTAssertEqual(CabalCopy.requestCount(2), "2 requests to join")
    }

    func testTheUnreadBadgeFormatterHidesZeroAndCapsAtNinetyNine() {
        XCTAssertNil(CabalCopy.unreadBadge(0))
        XCTAssertEqual(CabalCopy.unreadBadge(7), "7")
        XCTAssertEqual(CabalCopy.unreadBadge(99), "99")
        XCTAssertEqual(CabalCopy.unreadBadge(100), "99+")
        XCTAssertEqual(CabalCopy.unreadLabel(1), "1 unread message")
        XCTAssertEqual(CabalCopy.unreadLabel(3), "3 unread messages")
        XCTAssertEqual(CabalCopy.unreadLabel(100), "More than 99 unread messages")
    }

    func testTheV3CabalLines() {
        XCTAssertEqual(CabalCopy.roleLine(isCreator: true), "You’re an administrator")
        XCTAssertEqual(CabalCopy.roleLine(isCreator: false), "You’re a member")
        XCTAssertEqual(CabalCopy.votersSummary(members: 8, eligible: 8), "8 members · 8 eligible voters")
        XCTAssertEqual(CabalCopy.votersSummary(members: 1, eligible: 1), "1 member · 1 eligible voter")
        XCTAssertEqual(CabalCopy.requestsLine(1), "1 request to review")
        XCTAssertEqual(CabalCopy.requestsLine(2), "2 requests to review")
        XCTAssertEqual(CabalCopy.chatFooter(cabalName: "Big Ballers", unread: 0), "Big Ballers")
        XCTAssertEqual(CabalCopy.chatFooter(cabalName: "Big Ballers", unread: 1), "Big Ballers · 1 new message")
        XCTAssertEqual(CabalCopy.chatFooter(cabalName: "Big Ballers", unread: 120), "Big Ballers · 99+ new messages")
    }

    func testTheRulesCardReadsThresholdAndVoters() {
        var cabal = Components.Schemas.Cabal.sampleWithMembers(role: "creator")
        cabal.rules.threshold = "majority"
        XCTAssertEqual(CabalCopy.whoVotes(cabal), "Everyone")
        XCTAssertEqual(CabalCopy.thresholdLine(cabal), "2 of 3 eligible voters")
        cabal.rules.threshold = "unanimous"
        XCTAssertEqual(CabalCopy.thresholdLine(cabal), "3 of 3 eligible voters")
        cabal.rules.voterMode = "list"
        cabal.members[2].canVote = false
        XCTAssertEqual(CabalCopy.whoVotes(cabal), "People I pick")
        XCTAssertEqual(CabalCopy.eligibleVoterCount(cabal), 2)
        XCTAssertEqual(CabalCopy.thresholdLine(cabal), "2 of 2 eligible voters")
    }

    func testTheChatPreviewKeepsOnlyALiveMessageWithABody() {
        var message = Components.Schemas.ChatMessage(
            id: "m-1", author: .init(id: "u-1", handle: "alex", displayName: " ", photoUrl: nil), body: "gm",
            createdAt: Date(timeIntervalSince1970: 0), alsoInChannel: false, replyCount: 0, deleted: false)
        XCTAssertEqual(CabalChatPreviewMessage(message)?.author, "@alex")
        XCTAssertEqual(CabalChatPreviewMessage(message)?.body, "gm")
        message.body = nil
        XCTAssertNil(CabalChatPreviewMessage(message))
        message.body = "gm"
        message.deleted = true
        XCTAssertNil(CabalChatPreviewMessage(message))
    }

    private func myCabal(role: String, pending: Int32) -> Components.Schemas.MyCabal {
        .init(
            id: "c-1", name: "QA pot", pictureUrl: nil, role: role, canVote: true, memberCount: 2,
            joinedAt: Date(timeIntervalSince1970: 0), pendingRequestCount: pending, unreadCount: 0)
    }

    private func member(displayName: String, handle: String?, role: String) -> Components.Schemas.CabalMember {
        .init(
            userId: "u-1", handle: handle, displayName: displayName, photoUrl: nil, role: role, canVote: true,
            joinedAt: Date(timeIntervalSince1970: 0))
    }
}
