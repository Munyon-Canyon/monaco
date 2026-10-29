import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class CabalInviteTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_000_000)
    private let hour: TimeInterval = 60 * 60

    func testHandleDropsTheAtSignAndLowercases() {
        XCTAssertEqual(CabalInvite.handle("@Alex"), "alex")
    }

    func testHandleTrimsWhitespaceAroundTheAtSign() {
        XCTAssertEqual(CabalInvite.handle("  @QA_b \n"), "qa_b")
        XCTAssertEqual(CabalInvite.handle("@ kai"), "kai")
    }

    func testHandleWithoutAnAtSignIsKept() {
        XCTAssertEqual(CabalInvite.handle("kai"), "kai")
    }

    func testHandleOfOnlyAnAtSignIsEmpty() {
        XCTAssertEqual(CabalInvite.handle(" @ "), "")
    }

    func testSevenDaysLeftReadsSevenDays() {
        XCTAssertEqual(CabalInvite.expiryText(expiresAt: now + 7 * 24 * hour, now: now), "Expires in 7 days")
    }

    func testAFreshInviteJustUnderSevenDaysStillReadsSevenDays() {
        XCTAssertEqual(CabalInvite.expiryText(expiresAt: now + 7 * 24 * hour - 5, now: now), "Expires in 7 days")
    }

    func testTwentyFiveHoursRoundsUpToTwoDays() {
        XCTAssertEqual(CabalInvite.expiryText(expiresAt: now + 25 * hour, now: now), "Expires in 2 days")
    }

    func testExactlyOneDayReadsOneDay() {
        XCTAssertEqual(CabalInvite.expiryText(expiresAt: now + 24 * hour, now: now), "Expires in 1 day")
    }

    func testTwentyThreeHoursReadsToday() {
        XCTAssertEqual(CabalInvite.expiryText(expiresAt: now + 23 * hour, now: now), "Expires today")
    }

    func testAnInvitePastItsExpiryReadsExpired() {
        XCTAssertEqual(CabalInvite.expiryText(expiresAt: now - hour, now: now), "Expired")
        XCTAssertEqual(CabalInvite.expiryText(expiresAt: now, now: now), "Expired")
    }

    func testOnlyTheCreatorCanInvite() {
        XCTAssertTrue(CabalInviteStanding(role: .creator).canInvite)
        XCTAssertFalse(CabalInviteStanding(role: .member).canInvite)
    }

    func testANonMemberCannotInvite() {
        XCTAssertFalse(CabalInviteStanding(role: nil).canInvite)
    }

    func testTheInviterAndTheCreatorCanRevoke() {
        let creator = CabalInviteStanding(role: .creator)
        XCTAssertTrue(creator.canRevoke(invitedBy: "viewer", viewerID: "viewer"))
        XCTAssertTrue(creator.canRevoke(invitedBy: "someone-else", viewerID: "viewer"))
        XCTAssertFalse(CabalInviteStanding(role: .member).canRevoke(invitedBy: "viewer", viewerID: "viewer"))
    }

    func testStandingReadsTheCabal() throws {
        let member = try decodeCabal(joinMode: "request", me: #"{"role":"member","can_vote":true,"can_propose":true}"#)
        XCTAssertEqual(CabalInviteStanding(member), CabalInviteStanding(role: .member))
        XCTAssertFalse(CabalInviteStanding(member).canInvite)

        let creator = try decodeCabal(
            joinMode: "request", me: #"{"role":"creator","can_vote":true,"can_propose":true}"#)
        XCTAssertTrue(CabalInviteStanding(creator).canInvite)

        let outsider = try decodeCabal(joinMode: "request", me: "null")
        XCTAssertEqual(CabalInviteStanding(outsider).role, nil)
        XCTAssertFalse(CabalInviteStanding(outsider).canInvite)
    }

    private func decodeCabal(joinMode: String, me: String) throws -> Components.Schemas.Cabal {
        let person =
            #"{"user_id":"00000000-0000-7000-8000-000000000001","handle":"kai","display_name":"Kai","photo_url":null}"#
        let json = """
            {"id":"00000000-0000-7000-8000-000000000002","name":"QA pot","picture_url":null,"status":"active",
            "rules":{"join_mode":"\(joinMode)","voter_mode":"all","threshold":"majority",
            "proposal_expiry_seconds":86400,"slippage_bps":100,"show_sides":false,"show_sides_since":null,"max_trade_bps":null,"who_can_propose":"voters"},
            "creator":\(person),"created_at":"2026-09-30T12:00:00Z","open_proposal_count":0,"voter_count":1,"member_count":2,"members":[],"me":\(me),"my_access_request":null,
            "invite_code":null,"treasury_address":"treasury-placeholder"}
            """
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(Components.Schemas.Cabal.self, from: Data(json.utf8))
    }
}
