import Testing

@testable import Monaco

@MainActor
struct UserProfileScreenTests {
    @Test func bothSlotsAreLive() {
        #expect(UserProfileHeaderSlot.isLive)
        #expect(UserProfileSharedCabalsSlot.isLive)
    }

    @Test func aRouteWithoutAPreviewStillBuilds() {
        let route = UserProfileRoute(userID: "u")
        #expect(route.preview == nil)
        #expect(route.destination().context == UserProfileContext(userID: "u"))
    }

    @Test func aRouteCarriesItsPreviewToTheScreen() {
        let preview = UserPreview(displayName: "Kai", handle: "kai", photoURL: nil)
        let screen = UserProfileRoute(userID: "u", preview: preview).destination()
        #expect(screen.context.preview == preview)
        #expect(
            screen.sections.map { String(describing: $0) } == ["UserProfileHeaderSlot", "UserProfileSharedCabalsSlot"])
    }
}
