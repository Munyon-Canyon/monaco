import Testing

@testable import Monaco

@MainActor
struct WithdrawRouteTests {
    @Test func everyWithdrawLinkOpensTheWithdrawScreen() {
        #expect(WithdrawRoute().destination() is WithdrawView)
    }
}
