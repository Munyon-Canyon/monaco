#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum UserProfileSampleState {
    case publicProfile, following, privateProfile, notFound, error
    case christelleActivity, christelleCabals, unranked, hiddenPerformance, blocked, viewer

    var data: UserProfileContentData {
        switch self {
        case .christelleActivity, .christelleCabals:
            UserProfileContentData(
                userID: "sample-christelle", title: "Christelle", crown: true,
                handleLine: "@christelle \u{00B7} #1 this month", bio: "Weeble refugee. I vote no a lot.",
                pnl: "+12.4%", rightCalls: "12 of 13", followers: 48, following: 21, cabals: 3, inCommon: 2)
        case .unranked:
            UserProfileContentData(
                userID: "sample-noor", title: "Noor", handleLine: "@noor \u{00B7} joined Oct 2026",
                rightCalls: "2 of 3", followers: 2, following: 3, cabals: 1, inCommon: 1)
        case .hiddenPerformance, .privateProfile:
            UserProfileContentData(
                userID: "sample-mia", title: "Mia Chen", handleLine: "@mia \u{00B7} #8 this month",
                bio: "Curious about good ideas and better questions.", pnl: "Private", rightCalls: "7 of 9",
                followers: 12, following: 8, cabals: 3, inCommon: 2, performanceHidden: true)
        case .blocked:
            UserProfileContentData(
                userID: "sample-mia", title: "Mia Chen", handleLine: "@mia \u{00B7} #8 this month", followers: 12,
                following: 8, cabals: 3, blockedByMe: true)
        case .viewer:
            UserProfileContentData(
                userID: "sample-me", title: "Logan Norman", handleLine: "@logan \u{00B7} joined Oct 2026",
                followers: 12, following: 8, cabals: 3, isViewer: true)
        default:
            UserProfileContentData(
                userID: "sample-mia", title: "Mia Chen", handleLine: "@mia \u{00B7} #8 this month",
                bio: "Curious about good ideas and better questions.", pnl: "+8.2%", rightCalls: "7 of 9",
                followers: 12, following: 8, cabals: 3, inCommon: 2, followedByMe: self == .following)
        }
    }

    var cabals: Components.Schemas.UserCabals {
        self == .hiddenPerformance || self == .privateProfile ? .sampleHiddenPerformance : .sample
    }

    var showsNavButtons: Bool { self != .notFound && self != .error && self != .viewer }

    var opensOnCabals: Bool { self == .christelleCabals }
}

struct UserProfileSampleScreen: View {
    let state: UserProfileSampleState
    @State private var selection = UserProfileTabsSlot.defaultSelection

    private static let activityRows: [MemberActivityRow] = [
        ("Voted", "Buy NVDA \u{00B7} AC Capital", "20m", MemberActivityRow.Glyph.voted),
        ("Proposed Buy META", "Degens \u{00B7} passed 4\u{2013}1", "5h", .proposed),
        ("\u{201C}Concentration is a feature\u{201D}", "on Buy NVDA $1,500 \u{00B7} AC Capital", "3d", .replied),
        ("Voted", "Buy SpaceX \u{00B7} Big Ballers", "4d", .voted),
        ("Topped the board", "September \u{00B7} 11 of 12 right", "3w", .toppedBoard),
    ].enumerated().map { index, item in
        MemberActivityRow(
            id: "\(index)", glyph: item.3, title: item.0, subtitle: item.1, time: item.2,
            destination: item.3 == .toppedBoard ? .board : .proposal("sample-proposal"))
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: MonacoTheme.Space.m) {
                    content
                }
                .padding(.top, MonacoTheme.Space.m)
            }
            .monacoCanvas()
            .navigationTitle("")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { if state.showsNavButtons { navButtons } }
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .onAppear { if state.opensOnCabals { selection = .cabals } }
    }

    @ToolbarContentBuilder private var navButtons: some ToolbarContent {
        ToolbarItem(placement: .topBarLeading) {
            MonacoGlassButton(systemImage: "chevron.left", accessibilityLabel: "Back", placement: .toolbar) {}
        }
        ToolbarItem(placement: .topBarTrailing) {
            MonacoGlassButton(
                systemImage: MonacoIcon.share.symbol, accessibilityLabel: "Share", placement: .toolbar
            ) {}
        }
        ToolbarItem(placement: .topBarTrailing) {
            Menu {
                Button {
                } label: {
                    Label(
                        UserProfileMoreMenu.SafetyAction.report.title,
                        systemImage: UserProfileMoreMenu.SafetyAction.report.systemImage)
                }
                Button(role: UserProfileMoreMenu.SafetyAction.block.role) {
                } label: {
                    Label(
                        UserProfileMoreMenu.SafetyAction.block.title,
                        systemImage: UserProfileMoreMenu.SafetyAction.block.systemImage)
                }
            } label: {
                Image(systemName: "ellipsis")
                    .font(.title3.weight(.medium))
                    .foregroundStyle(MonacoTheme.primaryText)
                    .frame(width: MonacoButtonMetrics.hitTarget, height: MonacoButtonMetrics.hitTarget)
                    .background(Circle().fill(MonacoTheme.surfaceSunken))
                    .contentShape(Circle())
            }
            .accessibilityLabel("More")
        }
    }

    @ViewBuilder private var content: some View {
        switch state {
        case .notFound:
            UserProfileNotFound {}
        case .error:
            UserProfileLoadFailed {}
        default:
            UserProfileContent(data: state.data, actions: UserProfileActions())
            if UserProfileTabsSlot.isVisible(phase: .loaded, blockedByMe: state.data.blockedByMe) {
                UserProfileTabsContent(
                    cabals: .loaded(state.cabals.cabals.map(UserCabalRow.init)),
                    activity: .loaded(Self.activityRows), cabalCount: state.cabals.cabals.count,
                    hasMoreActivity: false, selection: $selection)
            }
        }
    }
}
#endif
