import Foundation
import Testing

@testable import Monaco

struct DepositDeepLinkTests {
    @Test func theRedirectGivesTheSessionID() throws {
        let url = try #require(URL(string: "monaco://deposit/complete?session=01890A5D-AC96-774B-BCCE-B302099A8057"))
        #expect(DepositDeepLink.sessionID(in: url) == "01890a5d-ac96-774b-bcce-b302099a8057")
    }

    @Test func aWrongHostGivesNothing() throws {
        let url = try #require(URL(string: "monaco://referral/complete?session=01890a5d-ac96-774b-bcce-b302099a8057"))
        #expect(DepositDeepLink.sessionID(in: url) == nil)
    }

    @Test func aNonUUIDSessionGivesNothing() throws {
        let url = try #require(URL(string: "monaco://deposit/complete?session=not-a-uuid"))
        #expect(DepositDeepLink.sessionID(in: url) == nil)
    }
}
