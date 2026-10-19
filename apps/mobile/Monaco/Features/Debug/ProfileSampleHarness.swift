#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

enum ProfileSampleScenario: String, CaseIterable {
    case placeholder
    case photo
    case ready
    case facePicker
    case cabals
    case empty
    case loading
    case error
    case personPublic
    case personFollowing
    case personPrivate
    case personNotFound
    case personError
    case editProfileReady
    case editProfileInvalid
    case editProfileSaving
    case editProfileSaved
    case editProfileSaveFailure
    case editProfileError

    static func matching(_ arguments: [String]) -> ProfileSampleScenario? {
        guard let flag = arguments.firstIndex(of: "-MonacoProfileSample"),
            arguments.indices.contains(flag + 1)
        else { return nil }
        return ProfileSampleScenario(rawValue: arguments[flag + 1])
    }

    var personState: UserProfileSampleState? {
        switch self {
        case .personPublic: .publicProfile
        case .personFollowing: .following
        case .personPrivate: .privateProfile
        case .personNotFound: .notFound
        case .personError: .error
        default: nil
        }
    }

    var editPresets: EditProfilePresets? {
        let bio = "Building things. Asking better questions."
        var presets = EditProfilePresets(bioDraft: bio, bioEnabled: true, sessions: .handlePreview(.available))
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

enum UserProfileSampleState {
    case publicProfile, following, privateProfile, notFound, error
}

struct UserProfileSampleScreen: View {
    let state: UserProfileSampleState

    private var fixture: UserProfileFixture {
        UserProfileFixture(
            bio: "Curious about good ideas and better questions.",
            performance: state == .privateProfile
                ? .hidden : .figure(dollars: "+$984.00", percent: "+8.2%"),
            periodText: "1M", joinedText: "Joined October 2026", showsPublicActivity: true)
    }

    private var data: UserProfileContentData {
        UserProfileContentData(
            userID: "sample-mia", title: "Mia Chen", handle: "mia", photoURL: nil, followers: 12, following: 8,
            followedByMe: state == .following)
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
            .navigationTitle("Mia Chen")
            .navigationBarTitleDisplayMode(.inline)
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }

    @ViewBuilder private var content: some View {
        switch state {
        case .notFound:
            UserProfileNotFound {}
        case .error:
            UserProfileLoadFailed {}
        default:
            UserProfileContent(data: data, fixture: fixture, actions: UserProfileActions())
        }
    }
}

struct ProfileSampleHarness: View {
    let scenario: ProfileSampleScenario
    @ObservedObject var auth: PrivyAuthService
    @State private var store = AppSessionStore()

    var body: some View {
        SampleAppFrame(
            auth: auth, tab: .profile, routes: scenario.editPresets == nil ? [] : [EditProfileRoute()], store: store
        ) { Self.fill($0, for: scenario) }
        .environment(\.profileHeaderPresets, presets)
        .environment(\.editProfilePresets, scenario.editPresets ?? EditProfilePresets())
    }

    private var presets: ProfileHeaderPresets {
        ProfileHeaderPresets(
            showsFacePicker: scenario == .facePicker,
            bio: scenario == .ready || scenario == .photo ? "Building things. Asking better questions." : nil)
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

        let hasPhoto = scenario == .photo || scenario == .ready || scenario == .cabals
        session.profile = sampleProfile(
            userID: "sample-user",
            displayName: "Logan Norman",
            photoURL: hasPhoto ? samplePhotoURL : nil,
            createdAt: ISO8601DateFormatter().date(from: "2026-10-01T14:30:00Z")
        )

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

    static let samplePhotoURL = SampleImage.flat(.plum, name: "avatar")
}

private func apiMode(for scenario: ProfileSampleScenario) -> SampleAPIScript.Mode {
    switch scenario {
    case .empty: return .empty
    case .loading: return .hang
    case .error: return .offline
    default: return .populated
    }
}

final class ProfileSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ProfileSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(SampleAPIScript(mode: apiMode(for: scenario)))
        if let state = scenario.personState {
            return AnyView(UserProfileSampleScreen(state: state))
        }
        return AnyView(ProfileSampleHarness(scenario: scenario, auth: auth))
    }
}
#endif
