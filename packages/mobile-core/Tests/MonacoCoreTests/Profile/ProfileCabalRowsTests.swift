import MonacoAPI
import XCTest

@testable import MonacoCore

final class ProfileCabalRowsTests: XCTestCase {
    private func cabal(_ ref: Components.Schemas.CabalRef, role: String = "member", canVote: Bool = true)
        -> Components.Schemas.MyCabal
    {
        Components.Schemas.MyCabal(
            id: ref.id, name: ref.name, pictureUrl: nil, role: role, canVote: canVote, memberCount: 5,
            joinedAt: Date(timeIntervalSince1970: 0), pendingRequestCount: 0, unreadCount: 0)
    }

    private func members(_ rows: [ProfileCabalRow]) -> [ProfileCabalRow.Member] {
        rows.compactMap { if case .member(let member) = $0 { member } else { nil } }
    }

    func testRolesOwnershipAndTonesFollowThePortfolio() {
        let portfolio = Components.Schemas.MyPortfolio.sampleOwnProfile
        let cabals = [
            cabal(.sampleDegens), cabal(.sampleACCapital, role: "creator"),
            cabal(.sampleTechTitans, canVote: false),
        ]
        let rows = members(ProfileCabalRows.make(cabals: cabals, portfolio: portfolio, invites: []))
        XCTAssertEqual(rows.map(\.name), ["AC Capital", "Tech Titans", "Degens"])
        XCTAssertEqual(rows.map(\.subtitle), ["35% · admin", "12% · member", "8% · voter"])
        XCTAssertEqual(rows.map(\.value), ["$7,000.00", "$2,710.00", "$1,872.00"])
        XCTAssertEqual(rows.map(\.returnText), ["+4.7%", "\u{2212}1.7%", "+11.2%"])
        XCTAssertEqual(rows.map(\.returnTone), [.gain, .loss, .gain])
    }

    func testOwnershipRounding() {
        XCTAssertEqual(ProfileCabalRows.ownership(bps: 3550), "36%")
        XCTAssertEqual(ProfileCabalRows.ownership(bps: 850), "8.5%")
        XCTAssertEqual(ProfileCabalRows.ownership(bps: 800), "8%")
    }

    func testMembershipsWithoutAStakeStillShow() {
        let portfolio = Components.Schemas.MyPortfolio.sampleWithUnfunded
        let noSnapshot = Components.Schemas.CabalRef(id: "no-snapshot", name: "Fresh", pictureUrl: nil)
        let cabals = [cabal(.sampleACCapital), cabal(.sampleQuantKids), cabal(noSnapshot)]
        let rows = members(ProfileCabalRows.make(cabals: cabals, portfolio: portfolio, invites: []))
        XCTAssertEqual(rows.map(\.name), ["AC Capital", "Fresh", "Quant Kids"])
        XCTAssertEqual(rows.map(\.value), ["$7,000.00", "—", "—"])
        let cashedOut = Components.Schemas.MyPortfolio.sampleLoss
        let after = members(
            ProfileCabalRows.make(cabals: [cabal(.sampleBeta)], portfolio: cashedOut, invites: []))
        XCTAssertEqual(after.map(\.value), ["—"])
        XCTAssertNil(after.first?.returnText)
    }

    func testAnUnfundedMemberOfAValuedCabalReadsZeroDollars() {
        var portfolio = Components.Schemas.MyPortfolio.sampleLoss
        portfolio.cabals[1].potValueMicros = 5_000_000
        let rows = members(
            ProfileCabalRows.make(cabals: [cabal(.sampleBeta)], portfolio: portfolio, invites: []))
        XCTAssertEqual(rows.first?.value, "$0.00")
        XCTAssertNil(rows.first?.ownership)
    }

    func testInvitationsComeLastAndAreNotCounted() {
        let rows = ProfileCabalRows.make(
            cabals: [cabal(.sampleAlpha)], portfolio: nil, invites: Components.Schemas.CabalInvite.samples)
        XCTAssertEqual(ProfileCabalRows.memberCount(rows), 1)
        XCTAssertEqual(rows.count, 3)
        guard case .invited(let invite) = rows[1] else { return XCTFail("want an invitation") }
        XCTAssertEqual(invite.subtitle, "Invited by Kai Cenat · 4 members")
        XCTAssertNil(members(rows).first?.value)
    }
}
