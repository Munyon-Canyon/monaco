import Foundation
import MonacoAPI
import MonacoNotify
import XCTest

final class CatchUpCopyTests: XCTestCase {
    private typealias Copy = CatchUpCopy
    private let now = Components.Schemas.CatchUp.sampleNow
    private let six = Components.Schemas.CatchUp.six

    private var calendar: Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        calendar.locale = Locale(identifier: "en_US_POSIX")
        return calendar
    }

    func testSubLine() {
        XCTAssertEqual(Copy.subLine(count: 6), "6 to catch up · what you can still change comes first")
        XCTAssertEqual(Copy.subLine(count: 1), "1 to catch up · what you can still change comes first")
        XCTAssertEqual(Copy.joinedToast("Quant Kids"), "You're in Quant Kids.")
    }

    func testWaitingRow() throws {
        let item = try XCTUnwrap(six.waiting.first)
        let proposal = try XCTUnwrap(item.proposal)
        XCTAssertEqual(Copy.rowTitle(item), "Buy NVDA · $400")
        XCTAssertEqual(Copy.subtitle(item, now: now), "AC Capital · 5 of 8 voted · ends in 4h")
        XCTAssertEqual(Copy.tally(proposal).yes, "4")
        XCTAssertEqual(Copy.tally(proposal).no, "1")
    }

    func testEndsInUnits() {
        XCTAssertEqual(Copy.endsIn(now.addingTimeInterval(30), now: now), "ends now")
        XCTAssertEqual(Copy.endsIn(now.addingTimeInterval(25 * 60), now: now), "ends in 25m")
        XCTAssertEqual(Copy.endsIn(now.addingTimeInterval(2 * 86_400), now: now), "ends in 2d")
    }

    func testDollars() {
        XCTAssertEqual(Copy.dollars(micros: 1_234_500_000), "$1,234.50")
        XCTAssertEqual(Copy.dollars(micros: 1_050_000), "$1.05")
    }

    func testMentionAndReplyRows() throws {
        let mention = try XCTUnwrap(six.replies.first)
        XCTAssertEqual(Copy.rowTitle(mention), "Christelle mentioned you")
        XCTAssertEqual(Copy.subtitle(mention, now: now), "AC Capital · Buy NVDA")
        XCTAssertEqual(Copy.quote(mention)?.author, "Christelle")
        XCTAssertEqual(Copy.quote(mention)?.text, "Still out: @Andres, Jordan?")
        let reply = try XCTUnwrap(six.replies.last)
        XCTAssertEqual(Copy.rowTitle(reply), "Mia replied to you")
        XCTAssertEqual(Copy.subtitle(reply, now: now), "Tech Titans · Sell TSLA")
        XCTAssertEqual(Copy.quote(reply)?.author, "You")
    }

    func testPostReplyAndNoQuote() {
        let post = Components.Schemas.CatchUpItem(
            key: "chat:1", kind: .reply, unread: true, at: now,
            cabal: .init(id: "c", name: "Tech Titans", pictureUrl: nil),
            post: .init(feedItemId: "f", kind: "trade", label: "sale", actorName: "Logan"))
        XCTAssertEqual(Copy.subtitle(post, now: now), "Tech Titans · Logan's sale")
        XCTAssertNil(Copy.quote(post))
        XCTAssertEqual(Copy.rowTitle(post), "Someone replied to you")
        let anonymous = Components.Schemas.CatchUpItem(
            key: "chat:2", kind: .reply, unread: true, at: now,
            post: .init(feedItemId: "f", kind: "trade", label: "sale", actorName: ""))
        XCTAssertEqual(Copy.subtitle(anonymous, now: now), "sale")
        let bare = Components.Schemas.CatchUpItem(key: "chat:3", kind: .reply, unread: true, at: now)
        XCTAssertEqual(Copy.subtitle(bare, now: now), "")
    }

    func testInviteRow() throws {
        let invite = try XCTUnwrap(six.invites.first)
        XCTAssertEqual(Copy.rowTitle(invite), "Sam invited you to Quant Kids")
        XCTAssertEqual(Copy.subtitle(invite, now: now), "5 members · holds OpenAI")
        let bare = Components.Schemas.CatchUpItem(
            key: "invite:2", kind: .invite, unread: true, at: now,
            invite: .init(id: "i", memberCount: 1, topHoldingName: nil))
        XCTAssertEqual(Copy.subtitle(bare, now: now), "1 member")
        let none = Components.Schemas.CatchUpItem(key: "invite:3", kind: .invite, unread: true, at: now)
        XCTAssertEqual(Copy.subtitle(none, now: now), "")
        XCTAssertEqual(Copy.rowTitle(none), "Someone invited you to a cabal")
    }

    func testResultRows() throws {
        let passed = try XCTUnwrap(six.results.first)
        XCTAssertEqual(Copy.rowTitle(passed), "Buy META passed 4–1")
        XCTAssertEqual(Copy.subtitle(passed, now: now), "Degens · you voted Yes · filled")
        let failed = try XCTUnwrap(six.results.last)
        XCTAssertEqual(Copy.rowTitle(failed), "Sell TSLA failed 2–2")
        XCTAssertEqual(Copy.subtitle(failed, now: now), "Tech Titans · you voted No")
        let expired = Components.Schemas.CatchUpProposal(
            id: "p", kind: "buy", symbol: "SpaceXx", assetName: "SpaceX", assetLogoUrl: nil, usdcMicros: nil,
            tokenAmount: nil, proposer: .init(id: "u", name: "A", photoUrl: nil), createdAt: now, expiresAt: now,
            status: "closed", yes: 2, no: 0, voted: 2, eligible: 5, replyCount: 0, outcome: .expired)
        XCTAssertEqual(Copy.resultTitle(expired), "Buy SpaceX closed 2–0")
        var fillFailed = expired
        fillFailed.outcome = .fillFailed
        fillFailed.yes = 4
        fillFailed.no = 1
        XCTAssertEqual(Copy.resultTitle(fillFailed), "Buy SpaceX passed 4–1 · didn't go through")
        let rule = Components.Schemas.CatchUpItem(key: "proposal:r", kind: .result, unread: false, at: now)
        XCTAssertEqual(Copy.subtitle(rule, now: now), "")
        XCTAssertEqual(Copy.rowTitle(rule), "")
        XCTAssertEqual(Copy.subtitle(rule.withKind(.waitingVote), now: now), "")
    }

    func testRuleProposalTitle() {
        let rule = Components.Schemas.CatchUpProposal(
            id: "p", kind: "rule", symbol: "", assetName: "", assetLogoUrl: nil, usdcMicros: nil, tokenAmount: nil,
            proposer: .init(id: "u", name: "A", photoUrl: nil), createdAt: now, expiresAt: now, status: "open",
            yes: 0, no: 0, voted: 0, eligible: 1, replyCount: 0)
        XCTAssertEqual(Copy.proposalTitle(rule), "Rule change")
        XCTAssertEqual(Copy.waitingTitle(rule), "Rule change")
    }

    func testRelativeTimes() {
        func text(_ ago: TimeInterval) -> String {
            Copy.relativeTime(now.addingTimeInterval(-ago), now: now, calendar: calendar)
        }
        XCTAssertEqual(text(10), "now")
        XCTAssertEqual(text(20 * 60), "20m")
        XCTAssertEqual(text(5 * 3_600), "5h")
        XCTAssertEqual(text(3 * 86_400), "3d")
        let old = Date(timeIntervalSince1970: 1_760_227_200)
        XCTAssertEqual(Copy.relativeTime(old, now: now, calendar: calendar), "Oct 12")
    }

    private func cabals(_ count: Int) -> [Components.Schemas.CatchUpCabal] {
        Array(Components.Schemas.CatchUp.caughtUp.cabals.prefix(count))
            + (count > 3 ? [Components.Schemas.CatchUpCabal(id: "x", name: "Quant Kids", pictureUrl: nil)] : [])
    }

    func testCaughtUpLineNamesTheCabals() {
        let tail = "waits on you. The next vote lands here first."
        XCTAssertEqual(Copy.caughtUpLine(cabals: cabals(0)), "When a cabal you're in votes, it lands here first.")
        XCTAssertEqual(Copy.caughtUpLine(cabals: cabals(1)), "Nothing in AC Capital \(tail)")
        XCTAssertEqual(Copy.caughtUpLine(cabals: cabals(2)), "Nothing in AC Capital or Tech Titans \(tail)")
        XCTAssertEqual(
            Copy.caughtUpLine(cabals: cabals(3)), "Nothing in AC Capital, Tech Titans or Degens \(tail)")
        XCTAssertEqual(Copy.caughtUpLine(cabals: cabals(4)), "Nothing in your cabals \(tail)")
    }

    func testClearedReasons() throws {
        let cleared = Components.Schemas.CatchUp.caughtUp.cleared
        XCTAssertEqual(Copy.clearedSubtitle(cleared[0], now: now), "AC Capital · you voted Yes · filled")
        XCTAssertEqual(Copy.clearedSubtitle(cleared[1], now: now), "AC Capital · you replied")
        XCTAssertEqual(Copy.clearedSubtitle(cleared[2], now: now), "Tech Titans · Sell TSLA")
        XCTAssertEqual(Copy.clearedSubtitle(cleared[3], now: now), "5 members · later")
        var open = cleared[1]
        open.clearedReason = .voted
        XCTAssertEqual(Copy.clearedSubtitle(open, now: now), "AC Capital · you voted")
        open.proposal?.yourChoice = .no
        XCTAssertEqual(Copy.clearedSubtitle(open, now: now), "AC Capital · you voted No")
        open.clearedReason = .joined
        XCTAssertEqual(Copy.clearedSubtitle(open, now: now), "AC Capital · you joined")
        var invite = cleared[3]
        invite.clearedReason = .joined
        XCTAssertEqual(Copy.clearedSubtitle(invite, now: now), "you joined")
        invite.clearedReason = .cleared
        invite.invite = nil
        XCTAssertEqual(Copy.clearedSubtitle(invite, now: now), "later")
    }

    func testAccessibilityLabelReadsTheRow() throws {
        let mention = six.replies[0]
        XCTAssertEqual(
            Copy.accessibilityLabel(mention, now: now, calendar: calendar),
            "Christelle mentioned you in AC Capital, Buy NVDA: Still out, at Andres, Jordan? 20 minutes ago, unread.")
        let reply = six.replies[1]
        XCTAssertEqual(
            Copy.accessibilityLabel(reply, now: now, calendar: calendar),
            "Mia replied to you in Tech Titans, Sell TSLA: Deliveries miss is priced in. 3 days ago.")
        let waiting = six.waiting[0]
        XCTAssertEqual(
            Copy.accessibilityLabel(waiting, now: now, calendar: calendar),
            "Buy NVDA · $400, AC Capital · 5 of 8 voted · ends in 4h. 1 hour ago, unread.")
    }

    func testSpokenTime() {
        XCTAssertEqual(Copy.spokenTime(now, now: now, calendar: calendar), "just now")
        XCTAssertEqual(Copy.spokenTime(now.addingTimeInterval(-60), now: now, calendar: calendar), "1 minute ago")
        XCTAssertEqual(Copy.spokenTime(now.addingTimeInterval(-7_200), now: now, calendar: calendar), "2 hours ago")
        XCTAssertEqual(Copy.spokenTime(now.addingTimeInterval(-86_400), now: now, calendar: calendar), "1 day ago")
        let old = now.addingTimeInterval(-30 * 86_400)
        XCTAssertFalse(Copy.spokenTime(old, now: now, calendar: calendar).contains("ago"))
    }
}

extension Components.Schemas.CatchUpItem {
    fileprivate func withKind(_ kind: Components.Schemas.CatchUpItem.KindPayload) -> Self {
        .init(key: key, kind: kind, unread: unread, at: at)
    }
}
