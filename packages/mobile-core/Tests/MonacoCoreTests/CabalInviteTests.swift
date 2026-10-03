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

    func testAnInvitePastItsExpiryReadsToday() {
        XCTAssertEqual(CabalInvite.expiryText(expiresAt: now - hour, now: now), "Expires today")
    }

    func testEveryMemberOfAnOpenCabalCanInvite() {
        XCTAssertTrue(CabalInviteStanding(joinMode: .open, role: .member).canInvite)
        XCTAssertTrue(CabalInviteStanding(joinMode: .open, role: .creator).canInvite)
    }

    func testOnlyTheCreatorOfARequestCabalCanInvite() {
        XCTAssertTrue(CabalInviteStanding(joinMode: .request, role: .creator).canInvite)
        XCTAssertFalse(CabalInviteStanding(joinMode: .request, role: .member).canInvite)
    }

    func testANonMemberCannotInvite() {
        XCTAssertFalse(CabalInviteStanding(joinMode: .open, role: nil).canInvite)
        XCTAssertFalse(CabalInviteStanding(joinMode: .request, role: nil).canInvite)
    }

    func testTheInviterAndTheCreatorCanRevoke() {
        let member = CabalInviteStanding(joinMode: .open, role: .member)
        XCTAssertTrue(member.canRevoke(invitedBy: "viewer", viewerID: "viewer"))
        XCTAssertFalse(member.canRevoke(invitedBy: "someone-else", viewerID: "viewer"))
        XCTAssertFalse(member.canRevoke(invitedBy: "viewer", viewerID: nil))
        let creator = CabalInviteStanding(joinMode: .request, role: .creator)
        XCTAssertTrue(creator.canRevoke(invitedBy: "someone-else", viewerID: "viewer"))
    }

    func testStandingReadsTheCabal() throws {
        let requestMember = try decodeCabal(joinMode: "request", me: #"{"role":"member","can_vote":true}"#)
        XCTAssertEqual(CabalInviteStanding(requestMember), CabalInviteStanding(joinMode: .request, role: .member))
        XCTAssertFalse(CabalInviteStanding(requestMember).canInvite)

        let requestCreator = try decodeCabal(joinMode: "request", me: #"{"role":"creator","can_vote":true}"#)
        XCTAssertTrue(CabalInviteStanding(requestCreator).canInvite)

        let openMember = try decodeCabal(joinMode: "open", me: #"{"role":"member","can_vote":false}"#)
        XCTAssertTrue(CabalInviteStanding(openMember).canInvite)

        let outsider = try decodeCabal(joinMode: "open", me: "null")
        XCTAssertEqual(CabalInviteStanding(outsider).role, nil)
        XCTAssertFalse(CabalInviteStanding(outsider).canInvite)
    }

    func testAnUnknownJoinModeOnlyLetsTheCreatorInvite() throws {
        let cabal = try decodeCabal(joinMode: "waitlist", me: #"{"role":"member","can_vote":true}"#)
        XCTAssertFalse(CabalInviteStanding(cabal).canInvite)
    }

    private func decodeCabal(joinMode: String, me: String) throws -> Components.Schemas.Cabal {
        let person =
            #"{"user_id":"00000000-0000-7000-8000-000000000001","handle":"kai","display_name":"Kai","photo_url":null}"#
        let json = """
            {"id":"00000000-0000-7000-8000-000000000002","name":"QA pot","picture_url":null,"status":"active",
            "rules":{"join_mode":"\(joinMode)","voter_mode":"all","threshold":"majority",
            "proposal_expiry_seconds":86400,"slippage_bps":100},
            "creator":\(person),"member_count":2,"members":[],"me":\(me),"my_access_request":null,
            "invite_code":null,"treasury_address":"treasury-placeholder"}
            """
        return try JSONDecoder().decode(Components.Schemas.Cabal.self, from: Data(json.utf8))
    }
}
