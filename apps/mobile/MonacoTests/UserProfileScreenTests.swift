import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct UserProfileScreenTests {
    @Test func bothSlotsAreLive() {
        #expect(UserProfileHeaderSlot.isLive)
        #expect(UserProfileTabsSlot.isLive)
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
        #expect(screen.sections.map { String(describing: $0) } == ["UserProfileHeaderSlot", "UserProfileTabsSlot"])
    }

    @Test func theTabsOpenOnActivity() {
        #expect(UserProfileTabsSlot.defaultSelection == .activity)
    }

    @Test func theTabsShowOnlyOnceAProfileLoadedAndIsNotBlocked() {
        #expect(UserProfileTabsSlot.isVisible(phase: .loaded, blockedByMe: false))
        #expect(!UserProfileTabsSlot.isVisible(phase: .loaded, blockedByMe: true))
        for phase: UserProfileModel.Phase? in [nil, .idle, .loading, .unavailable, .failed] {
            #expect(!UserProfileTabsSlot.isVisible(phase: phase, blockedByMe: false))
        }
    }

    @Test func christelleWearsTheCrownAndHasFigures() {
        for state in [UserProfileSampleState.christelleActivity, .christelleCabals] {
            let data = state.data
            #expect(data.crown)
            #expect(data.handleLine == "@christelle \u{00B7} #1 this month")
            #expect(data.pnl == "+12.4%")
            #expect(data.rightCalls == "12 of 13")
            #expect(data.inCommon == 2)
            #expect(!data.blockedByMe && !data.isViewer && !data.performanceHidden)
        }
        #expect(UserProfileSampleState.christelleCabals.opensOnCabals)
        #expect(!UserProfileSampleState.christelleActivity.opensOnCabals)
    }

    @Test func anUnrankedMemberHasNoCrownAndNoReturn() {
        let data = UserProfileSampleState.unranked.data
        #expect(!data.crown)
        #expect(data.pnl == "\u{2014}")
        #expect(!data.handleLine.contains("this month"))
    }

    @Test func hiddenPerformanceReadsPrivateAndDropsThePercents() {
        let state = UserProfileSampleState.hiddenPerformance
        #expect(state.data.pnl == "Private")
        #expect(state.data.performanceHidden)
        #expect(state.cabals.cabals.map(UserCabalRow.init).allSatisfy { $0.ownership == nil })
    }

    @Test func aBlockedMemberHidesTheTabsAndAViewerSeesTheirOwnNotice() {
        #expect(UserProfileSampleState.blocked.data.blockedByMe)
        #expect(UserProfileSampleState.viewer.data.isViewer)
    }
}
