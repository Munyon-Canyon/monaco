#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

enum ProfileSampleScenario: String, CaseIterable {
    case placeholder
    case photo
    case ready
    case unranked
    case noStanding
    case hiddenPerformance
    case failed
    case facePicker
    case cabals
    case tabsCabals
    case tabsUnvalued
    case tabsActivity
    case tabsEmpty
    case tabsError
    case empty
    case loading
    case error
    case personPublic
    case personFollowing
    case personPrivate
    case personNotFound
    case personError
    case personChristelleActivity
    case personChristelleCabals
    case personUnranked
    case personHidden
    case personBlocked
    case personViewer
    case editProfileReady
    case editProfileInvalid
    case editProfileSaving
    case editProfileSaved
    case editProfileSaveFailure
    case editProfileError
    case invite

    static func matching(_ arguments: [String]) -> ProfileSampleScenario? {
        guard let flag = arguments.firstIndex(of: "-MonacoProfileSample"),
            arguments.indices.contains(flag + 1)
        else { return nil }
        return ProfileSampleScenario(rawValue: arguments[flag + 1])
    }

    static let sampleBio = "Building things. Asking better questions."

    var showsBio: Bool {
        switch self {
        case .ready, .invite, .unranked, .noStanding, .hiddenPerformance, .photo, .editProfileReady,
            .editProfileInvalid, .editProfileSaving, .editProfileSaved, .editProfileSaveFailure:
            true
        default:
            false
        }
    }

    var personState: UserProfileSampleState? {
        switch self {
        case .personPublic: .publicProfile
        case .personFollowing: .following
        case .personPrivate: .privateProfile
        case .personNotFound: .notFound
        case .personError: .error
        case .personChristelleActivity: .christelleActivity
        case .personChristelleCabals: .christelleCabals
        case .personUnranked: .unranked
        case .personHidden: .hiddenPerformance
        case .personBlocked: .blocked
        case .personViewer: .viewer
        default: nil
        }
    }

    var editPresets: EditProfilePresets? {
        var presets = EditProfilePresets(sessions: .handlePreview(.available))
        switch self {
        case .editProfileReady: break
        case .editProfileInvalid: presets.handleDraft = "a!"
        case .editProfileSaving:
            presets.nameDraft = "Logan N"
            presets.phase = .saving
        case .editProfileSaved: presets.phase = .saved
        case .editProfileSaveFailure:
            presets.nameDraft = "Logan N"
            presets.saveName = RejectingNameStore()
        case .editProfileError: presets.loadFailed = true
        default: return nil
        }
        return presets
    }
}

private struct RejectingNameStore: DisplayNameSaving {
    func saveDisplayName(_ draft: String) async -> ProfileSaveOutcome {
        .failed("Couldn't save your name. Try again.")
    }
}

struct ProfileTabsSampleScreen: View {
    let scenario: ProfileSampleScenario
    @State private var selection = ProfileTabsSelection.cabals

    private var cabals: LoadState<[ProfileCabalRow]> {
        switch scenario {
        case .tabsEmpty: .loaded([])
        case .tabsError: .failed(.transport(URLError(.notConnectedToInternet)))
        case .tabsUnvalued:
            .loaded(
                ProfileCabalRows.make(
                    cabals: [Self.cabal(.sampleACCapital), Self.cabal(.sampleQuantKids)],
                    portfolio: .sampleWithUnfunded, invites: []))
        default:
            .loaded(
                ProfileCabalRows.make(
                    cabals: [
                        Self.cabal(.sampleACCapital, role: "creator"), Self.cabal(.sampleTechTitans),
                        Self.cabal(.sampleDegens),
                    ], portfolio: .sampleOwnProfile,
                    invites: [Self.quantKidsInvite]))
        }
    }

    private static let activityRows: [MemberActivityRow] = [
        ("Voted Yes", "Buy NVDA · AC Capital", "20m", MemberActivityRow.Glyph.voted),
        ("Proposed Buy META", "AC Capital · passed 4–1", "5h", .proposed),
        ("“Concentration is a feature”", "on Buy NVDA $1,500.00 · Tech Titans", "3d", .replied),
        ("Voted No", "Sell TSLA · Degens", "1w", .voted),
        ("Topped the board", "September · 11 of 12 right", "3w", .toppedBoard),
    ].enumerated().map { index, item in
        MemberActivityRow(
            id: "\(index)", glyph: item.3, title: item.0, subtitle: item.1, time: item.2,
            destination: item.3 == .toppedBoard ? .board : .proposal("sample-proposal"))
    }

    private var activity: LoadState<[MemberActivityRow]> {
        switch scenario {
        case .tabsEmpty: .loaded([])
        case .tabsError: .failed(.transport(URLError(.notConnectedToInternet)))
        default:
            .loaded(Self.activityRows)
        }
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                ProfileTabsContent(
                    cabals: cabals, activity: activity, hasMoreActivity: false, selection: $selection)
            }
            .monacoCanvas()
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .onAppear { if scenario == .tabsActivity { selection = .activity } }
    }

    static func cabal(_ ref: Components.Schemas.CabalRef, role: String = "member")
        -> Components.Schemas.MyCabal
    {
        Components.Schemas.MyCabal(
            id: ref.id, name: ref.name, pictureUrl: nil, role: role, canVote: true, memberCount: 5,
            joinedAt: Date(timeIntervalSince1970: 0), pendingRequestCount: 0, unreadCount: 0)
    }

    static let quantKidsInvite = Components.Schemas.CabalInvite(
        requestId: "sample-invite",
        cabal: .init(
            id: Components.Schemas.CabalRef.sampleQuantKids.id, name: "Quant Kids", pictureUrl: nil, memberCount: 5),
        invitedBy: .init(userId: "sample-sam", handle: "sam", displayName: "Sam", photoUrl: nil),
        expiresAt: Date(timeIntervalSince1970: 1_791_590_400))
}

struct ProfileSampleHarness: View {
    let scenario: ProfileSampleScenario
    @ObservedObject var auth: PrivyAuthService
    @State private var store = AppSessionStore()

    var body: some View {
        SampleAppFrame(
            auth: auth, tab: .profile, routes: scenario.editPresets == nil ? [] : [EditProfileRoute()], store: store,
            session: { Self.fill($0, for: scenario) },
            sheet: scenario == .invite
                ? { AnyView(CabalInviteSheet(cabal: .sample(role: "creator"), mode: .share)) } : nil
        )
        .environment(\.profileHeaderPresets, presets)
        .environment(\.editProfilePresets, scenario.editPresets ?? EditProfilePresets())
    }

    private var presets: ProfileHeaderPresets {
        ProfileHeaderPresets(
            showsFacePicker: scenario == .facePicker)
    }

    private static func fill(_ session: AppSessionStore, for scenario: ProfileSampleScenario) {
        session.isLoading = false
        switch scenario {
        case .loading:
            session.isLoading = true
            return
        case .error:
            session.errorMessage = "Couldn't connect to Monaco."
            return
        default:
            break
        }

        let hasPhoto = scenario == .photo || scenario == .ready || scenario == .cabals || scenario == .invite
        session.profile = sampleProfile(
            userID: "sample-user",
            displayName: "Logan Norman",
            photoURL: hasPhoto ? samplePhotoURL : nil,
            createdAt: ISO8601DateFormatter().date(from: "2026-10-01T14:30:00Z")
        )
        if scenario.showsBio { session.profile?.bio = ProfileSampleScenario.sampleBio }

        session.hasLoaded = true
    }

    static func sampleProfile(userID: String, displayName: String, photoURL: URL?, createdAt: Date? = nil)
        -> SessionProfile
    {
        var profile = SessionProfile(Components.Schemas.Me.sample)
        profile.userID = userID
        profile.displayName = displayName
        profile.photoURL = photoURL
        if let createdAt { profile.createdAt = createdAt }
        return profile
    }

    static let samplePhotoURL = SampleImage.flat(.purple, name: "avatar")
}

private func apiScript(for scenario: ProfileSampleScenario) -> SampleAPIScript {
    var script = SampleAPIScript(mode: apiMode(for: scenario))
    script.user.returnPrivate = scenario == .hiddenPerformance
    if scenario.showsBio { script.user.bio = ProfileSampleScenario.sampleBio }
    switch scenario {
    case .unranked: script.user.standing = .sampleUnranked
    case .noStanding: script.user.standing = nil
    default: break
    }
    if scenario == .ready {
        script.balance.inFlightMicros = "0"
        script.balance.onChainMicros = script.balance.availableMicros
        script.homeCabals = [
            ProfileTabsSampleScreen.cabal(.sampleACCapital, role: "creator"),
            ProfileTabsSampleScreen.cabal(.sampleTechTitans),
            ProfileTabsSampleScreen.cabal(.sampleDegens),
        ]
        script.portfolio = .sampleOwnProfile
        script.invites = [ProfileTabsSampleScreen.quantKidsInvite]
    }
    return script
}

private func apiMode(for scenario: ProfileSampleScenario) -> SampleAPIScript.Mode {
    switch scenario {
    case .empty: return .empty
    case .loading: return .hang
    case .error, .failed: return .offline
    default: return .populated
    }
}

final class ProfileSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ProfileSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(apiScript(for: scenario))
        switch scenario {
        case .tabsCabals, .tabsUnvalued, .tabsActivity, .tabsEmpty, .tabsError:
            return AnyView(ProfileTabsSampleScreen(scenario: scenario))
        default:
            break
        }
        if let state = scenario.personState {
            return AnyView(UserProfileSampleScreen(state: state))
        }
        return AnyView(ProfileSampleHarness(scenario: scenario, auth: auth))
    }
}
#endif
