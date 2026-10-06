import Foundation
import MonacoAPI
import XCTest

final class NullableReferenceDecodingTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8058"

    private func cabal(me: String, accessRequest: String) -> String {
        ##"{"id":"\##(cabalID)","name":"Kai","picture_url":null,"status":"active","##
            + ##""rules":{"join_mode":"open","voter_mode":"all","threshold":"majority","##
            + ##""proposal_expiry_seconds":86400,"slippage_bps":100},"##
            + ##""creator":{"user_id":"\##(cabalID)","handle":"kai","display_name":"Kai","photo_url":null},"##
            + ##""member_count":1,"members":[],"me":\##(me),"my_access_request":\##(accessRequest),"##
            + ##""invite_code":"ABCD2345","treasury_address":"Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf"}"##
    }

    private func proposal(myBallot: String) -> String {
        ##"{"id":"01890a5d-ac96-774b-bcce-b302099a8057","cabal_id":"\##(cabalID)","##
            + ##""proposer_id":"01890a5d-ac96-774b-bcce-b302099a8059","kind":"buy","symbol":"AAPLx","##
            + ##""usdc_micros":25000000,"token_amount":null,"quote_out_amount":105000000,"thesis":null,"##
            + ##""status":"open","status_reason":null,"status_message":null,"##
            + ##""expires_at":"2026-10-04T15:00:00Z","created_at":"2026-10-03T15:00:00Z","##
            + ##""tally":{"yes":1,"no":0,"voters":3,"needed":2},"my_ballot":\##(myBallot),"can_vote":false}"##
    }

    private func decode<T: Decodable>(_: T.Type, _ raw: String) throws -> T {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(T.self, from: Data(raw.utf8))
    }

    func testACabalDecodesTheCallersMembershipAndAccessRequest() throws {
        let raw = cabal(
            me: #"{"role":"creator","can_vote":true}"#,
            accessRequest: #"{"id":"01890a5d-ac96-774b-bcce-b302099a8059","direction":"request","status":"pending"}"#
        )

        let decoded = try decode(Components.Schemas.Cabal.self, raw)

        XCTAssertEqual(decoded.me?.role, "creator")
        XCTAssertEqual(decoded.me?.canVote, true)
        XCTAssertEqual(decoded.myAccessRequest?.direction, "request")
    }

    func testACabalSeenByANonMemberDecodesWithNullMembership() throws {
        let decoded = try decode(Components.Schemas.Cabal.self, cabal(me: "null", accessRequest: "null"))

        XCTAssertNil(decoded.me)
        XCTAssertNil(decoded.myAccessRequest)
    }

    func testAProposalDecodesTheCallersBallot() throws {
        let decoded = try decode(Components.Schemas.Proposal.self, proposal(myBallot: #""yes""#))

        XCTAssertEqual(decoded.myBallot, .yes)
    }

    func testAProposalTheCallerHasNotVotedOnDecodesWithNoBallot() throws {
        let decoded = try decode(Components.Schemas.Proposal.self, proposal(myBallot: "null"))

        XCTAssertNil(decoded.myBallot)
    }
}
