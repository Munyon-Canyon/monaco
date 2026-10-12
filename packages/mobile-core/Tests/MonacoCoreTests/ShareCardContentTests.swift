import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class ShareCardContentTests: XCTestCase {
    private typealias Cabal = Components.Schemas.Cabal
    private typealias Pot = Components.Schemas.CabalPot

    private static let utc = TimeZone(identifier: "UTC") ?? .current
    private static let english = Locale(identifier: "en_US")
    private static let unlocked = InviteLinks(Components.Schemas.MyReferralCode.sample)
    private static let locked = InviteLinks(Components.Schemas.MyReferralCode.sampleLocked)

    func testCabalGainCardReadsTheGainAndTheReturnFromThePot() throws {
        let content = try cabal(Cabal.sampleWithMembers(role: "member"), pot: Pot.sampleInvested)

        XCTAssertEqual(content.kind, .cabal)
        XCTAssertEqual(content.title, "QA pot")
        XCTAssertEqual(content.subtitle, "3 members")
        XCTAssertEqual(content.gain, "+$0.73")
        XCTAssertEqual(content.returnText, "+0.1%")
        XCTAssertEqual(content.direction, .up)
        XCTAssertEqual(content.dateText, "Oct 3, 2026")
        XCTAssertEqual(
            content.stats,
            [
                .init(label: "Put in", value: "$999.27"),
                .init(label: "In the pot", value: "$1,000.00"),
                .init(label: "Members", value: "3"),
            ])
    }

    func testTheCurveIsTheGainSeriesNotTheValueSeries() throws {
        let history = Components.Schemas.CabalValueHistory.sample()
        let content = try cabal(Cabal.sampleWithMembers(role: "member"), pot: Pot.sampleInvested, history: history)

        XCTAssertEqual(content.points.map(\.value), [0, 5_000_000, -2_000_000, 10_000_000])
        XCTAssertEqual(content.points.map(\.at), history.points.map(\.at))
    }

    func testTheStatusIsOpenOrByRequest() throws {
        var open = Cabal.sampleWithMembers(role: "member")
        open.rules.joinMode = "open"

        XCTAssertEqual(try cabal(open, pot: Pot.sampleInvested).status, "Open")
        XCTAssertEqual(try cabal(Cabal.sampleWithMembers(role: "member"), pot: Pot.sampleInvested).status, "By request")
    }

    func testCabalLossCardIsNegativeAndDown() throws {
        let pot = Pot.sample(
            potValueMicros: 998_000_000, cashMicros: 998_000_000, cashWeightBps: 10000, pnlMicros: -2_000_000,
            returnBps: -40, holdings: [], me: nil)

        let content = try cabal(Cabal.sampleWithMembers(role: "member"), pot: pot)

        XCTAssertEqual(content.gain, "\u{2212}$2.00")
        XCTAssertEqual(content.returnText, "\u{2212}0.4%")
        XCTAssertEqual(content.direction, .down)
        XCTAssertEqual(content.stats.first?.value, "$1,000.00")
    }

    func testABannedCabalHasNoCard() {
        let content = ShareCardContent.cabal(
            Cabal.sampleBanned(role: "member"), pot: Pot.sampleInvested,
            history: .sample(), links: Self.unlocked, locale: Self.english, timeZone: Self.utc)

        XCTAssertNil(content)
    }

    func testANullReturnIsADash() throws {
        let pot = Pot.sample(
            potValueMicros: 0, cashMicros: 0, cashWeightBps: 0, pnlMicros: 0, returnBps: nil, holdings: [], me: nil)

        let content = try cabal(Cabal.sampleWithMembers(role: "member"), pot: pot)

        XCTAssertEqual(content.returnText, "—")
        XCTAssertEqual(content.gain, "$0.00")
        XCTAssertEqual(content.direction, .flat)
    }

    func testPutInIsThePotMinusTheGainAndIsTheMembersNetContributedForASoleMember() throws {
        let sole = Pot.sample(
            potValueMicros: 380_150_000, cashMicros: 380_150_000, cashWeightBps: 10000, pnlMicros: 150_000,
            returnBps: 4, holdings: [],
            me: .init(
                shareUnits: 380_000_000, valueMicros: 380_150_000, sliceBps: 10000, netContributedMicros: 380_000_000,
                pnlMicros: 150_000))

        let content = try cabal(Cabal.sampleWithMembers(role: "member"), pot: sole)

        XCTAssertEqual(
            content.stats.first?.value, UsdAmountFormatter.format(micros: sole.potValueMicros - sole.pnlMicros))
        XCTAssertEqual(
            content.stats.first?.value, UsdAmountFormatter.format(micros: sole.me?.netContributedMicros ?? -1))
    }

    func testAnEmptyHistoryStillGivesACompleteCard() throws {
        var history = Components.Schemas.CabalValueHistory.sample()
        history.points = []

        let content = try cabal(Cabal.sampleWithMembers(role: "member"), pot: Pot.sampleInvested, history: history)

        XCTAssertTrue(content.points.isEmpty)
        XCTAssertEqual(content.gain, "+$0.73")
        XCTAssertEqual(
            content.spoken,
            "QA pot, by request cabal, 3 members. Up $0.73, 0.1 percent, all time. Put in $999.27, in the pot $1,000.00."
        )
    }

    func testMeCardReadsThePortfolio() throws {
        let content = me(.sample)

        XCTAssertEqual(content.kind, .me)
        XCTAssertEqual(content.title, "Kai Cenat")
        XCTAssertEqual(content.subtitle, "@kaicenat")
        XCTAssertEqual(content.status, "Member since Sep 2025")
        XCTAssertEqual(content.dateText, "Oct 3, 2026")
        XCTAssertEqual(content.gain, "+$14.00")
        XCTAssertEqual(content.returnText, "+1.4%")
        XCTAssertEqual(content.direction, .up)
        XCTAssertEqual(content.points.map(\.value), [0, 1_500_000, 3_000_000, 4_500_000])
        XCTAssertEqual(
            content.stats,
            [
                .init(label: "Put in", value: "$986.00"),
                .init(label: "In cabals", value: "$1,000.00"),
                .init(label: "Cabals", value: "2"),
            ])
        XCTAssertEqual(
            content.spoken,
            "Kai Cenat, @kaicenat. Up $14.00, 1.4 percent, all time. Put in $986.00, in cabals $1,000.00.")
    }

    func testMeCardWithNothingInvestedIsZeroWithADashReturn() {
        let content = me(.sampleEmpty)

        XCTAssertEqual(content.gain, "$0.00")
        XCTAssertEqual(content.returnText, "—")
        XCTAssertEqual(content.direction, .flat)
        XCTAssertNil(content.dateText)
        XCTAssertEqual(content.stats.last, .init(label: "Cabals", value: "0"))
    }

    func testMeLossCard() {
        let content = me(.sampleLoss)

        XCTAssertEqual(content.gain, "\u{2212}$1.00")
        XCTAssertEqual(content.direction, .down)
    }

    func testAUnlockedHandleIsTheFooterAndTheShareLink() throws {
        let content = try cabal(Cabal.sampleWithMembers(role: "member"), pot: Pot.sampleInvested)

        XCTAssertEqual(content.footerLink, "monacolabs.xyz/r/kaicenat")
        XCTAssertEqual(content.shareURL, URL(string: Components.Schemas.MyReferralCode.sample.handleLink ?? ""))
    }

    func testALockedHandleSharesTheCodeLink() throws {
        let content = try cabal(
            Cabal.sampleWithMembers(role: "member"), pot: Pot.sampleInvested, links: Self.locked)

        XCTAssertEqual(content.footerLink, "monacolabs.xyz/r/k7m4qx2p")
        XCTAssertEqual(content.shareURL, URL(string: "https://monacolabs.xyz/r/k7m4qx2p"))
    }

    func testNoCodeFallsBackToTheSiteWithoutBlocking() {
        let content = me(.sample, links: nil)

        XCTAssertEqual(content.footerLink, "monacolabs.xyz")
        XCTAssertEqual(content.shareURL, URL(string: "https://monacolabs.xyz"))
    }

    func testAOneMemberCabalIsSingular() throws {
        var solo = Cabal.sample(role: "creator")
        solo.memberCount = 1

        XCTAssertEqual(try cabal(solo, pot: Pot.sampleInvested).subtitle, "1 member")
    }

    private func cabal(
        _ cabal: Cabal, pot: Pot, history: Components.Schemas.CabalValueHistory = .sample(),
        links: InviteLinks? = ShareCardContentTests.unlocked
    ) throws -> ShareCardContent {
        try XCTUnwrap(
            ShareCardContent.cabal(
                cabal, pot: pot, history: history, links: links, locale: Self.english, timeZone: Self.utc))
    }

    private func me(
        _ portfolio: Components.Schemas.MyPortfolio, links: InviteLinks? = ShareCardContentTests.unlocked
    ) -> ShareCardContent {
        ShareCardContent.me(
            profile: SessionProfile(Components.Schemas.Me.sample), portfolio: portfolio, history: .sample(range: .all),
            links: links, locale: Self.english, timeZone: Self.utc)
    }
}
