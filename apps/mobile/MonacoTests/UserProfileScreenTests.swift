import MonacoCore
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

    @Test func sharedCabalsShowOnlyOnceAnotherMembersProfileLoaded() {
        func visible(_ phase: UserProfileModel.Phase?) -> Bool {
            UserProfileSharedCabalsSlot.isVisible(phase: phase, userID: "u", viewerID: "me")
        }
        #expect(visible(.loaded))
        for phase: UserProfileModel.Phase? in [nil, .idle, .loading, .unavailable, .failed] {
            #expect(!visible(phase))
        }
        #expect(!UserProfileSharedCabalsSlot.isVisible(phase: .loaded, userID: "me", viewerID: "me"))
    }
}
