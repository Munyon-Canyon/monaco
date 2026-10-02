import Foundation
import MonacoAPI
import XCTest

final class CabalMemberDecodingTests: XCTestCase {
    func testAMemberDecodesPersonFieldsAndVoteStandingTogether() throws {
        let id = "01890a5d-ac96-774b-bcce-b302099a8058"
        let raw =
            ##"{"user_id":"\##(id)","handle":"kai","display_name":"Kai","##
            + ##""photo_url":null,"role":"creator","can_vote":true,"joined_at":"2026-09-30T12:00:00Z"}"##
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let member = try decoder.decode(Components.Schemas.CabalMember.self, from: Data(raw.utf8))
        XCTAssertEqual(member.role, "creator")
        XCTAssertEqual(member.userId, id)
    }
}
