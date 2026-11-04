import MonacoAPI
import MonacoCore
import MonacoTestSupport
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
            #expect(data.shared?.marks.map(\.name) == ["AC Capital", "Degens"])
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

    @Test func anUnrankedMemberSharesNoCabal() {
        #expect(UserProfileSampleState.unranked.data.shared == nil)
    }

    @Test func theSharedLineReadsAsASentenceToVoiceOver() {
        let both = UserProfileSharedData(marks: [], text: "Both in AC Capital and Degens")
        let more = UserProfileSharedData(marks: [], text: "In A, B and 3 more")
        #expect(both.accessibilityLabel == "You are both in AC Capital and Degens")
        #expect(more.accessibilityLabel == "You are both in A, B and 3 more")
    }

    @Test func theFigureLabelsAreTheSharedCopy() {
        #expect(ProfileCopy.monthLabel == "This month")
        #expect(ProfileCopy.rightMonthLabel == "Right this month")
    }

    @Test func oneLoadOfTheSharedModelIsOneRequestAndFeedsTheHeaderLine() async throws {
        let body = String(decoding: try JSONEncoder().encode(Components.Schemas.UserCabals.sample), as: UTF8.self)
        let transport = StubTransport(scripted: [.json(.ok, body)])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "t"), transport: transport)
        let model = UserCabalsModel(userID: "user-2", viewerID: "viewer", api: api)
        await model.load()
        let paths = await transport.sent.map(\.path)
        #expect(paths == ["/v1/users/user-2/cabals"])
        let line = try #require(model.sharedLine.map(UserProfileSharedData.init))
        #expect(line.text == "Both in AC Capital and Degens")
        #expect(line == UserProfileSharedData.sampleBoth)
    }
}
