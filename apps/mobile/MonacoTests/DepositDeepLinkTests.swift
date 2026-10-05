import Foundation
import Testing

@testable import Monaco

struct DepositDeepLinkTests {
    @Test func theRedirectOpensTheCompletionOnHome() throws {
        let url = try #require(URL(string: "monaco://deposit/complete?session=01890A5D-AC96-774B-BCCE-B302099A8057"))

        let (route, tab) = try #require(DepositDeepLink.route(for: url))

        #expect(tab == .home)
        #expect(
            (route as? DepositCompleteRoute) == DepositCompleteRoute(sessionID: "01890a5d-ac96-774b-bcce-b302099a8057"))
    }

    @Test func aWrongHostOpensNothing() throws {
        let url = try #require(URL(string: "monaco://referral/complete?session=01890a5d-ac96-774b-bcce-b302099a8057"))
        #expect(DepositDeepLink.route(for: url) == nil)
    }

    @Test func aNonUUIDSessionOpensNothing() throws {
        let url = try #require(URL(string: "monaco://deposit/complete?session=not-a-uuid"))
        #expect(DepositDeepLink.route(for: url) == nil)
    }
}
