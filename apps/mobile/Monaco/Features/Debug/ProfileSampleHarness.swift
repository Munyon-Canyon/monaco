#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

/// Debug-only: renders Profile from canned `AppSessionStore` data so QA can screenshot
/// each state without Privy or a backend. Launch with
/// `-MonacoProfileSample <placeholder|photo|facePicker|validation|saveFailure|saveSuccess|cabals|empty|loading|error>`.
/// `cabals` and `empty` open scrolled to the bottom so the cabal list is on screen.
enum ProfileSampleScenario: String, CaseIterable {
    case placeholder
    case photo
    /// The face sheet open over the placeholder, so the grid and its ring can be screenshotted.
    case facePicker
    case validation
    /// Edit profile open on a valid new name. There is no session here, so tapping Save is
    /// a rejected save — which is how the failure is meant to be readable inside the sheet.
    case saveFailure
    /// The same sheet with a store that accepts the save, so the other half of the fix — the
    /// sheet closing first and the toast landing on the uncovered screen — can be seen.
    case saveSuccess
    case cabals
    case empty
    case loading
    case error

    static func matching(_ arguments: [String]) -> ProfileSampleScenario? {
        guard let flag = arguments.firstIndex(of: "-MonacoProfileSample"),
            arguments.indices.contains(flag + 1)
        else { return nil }
        return ProfileSampleScenario(rawValue: arguments[flag + 1])
    }
}

struct ProfileSampleHarness: View {
    let scenario: ProfileSampleScenario
    @ObservedObject var auth: PrivyAuthService
    @State private var store = AppSessionStore()

    var body: some View {
        SampleAppFrame(auth: auth, tab: .profile, store: store) { Self.fill($0, for: scenario) }
            .defaultScrollAnchor(scenario == .cabals || scenario == .empty ? .bottom : .top)
            .environment(\.profileHeaderPresets, presets)
    }

    private var presets: ProfileHeaderPresets {
        ProfileHeaderPresets(
            nameDraft: Self.nameDraft(for: scenario),
            showsEditProfile: scenario == .validation || scenario == .saveFailure || scenario == .saveSuccess,
            showsFacePicker: scenario == .facePicker,
            saveName: saveNameOverride
        )
    }

    private var saveNameOverride: (any DisplayNameSaving)? {
        scenario == .saveSuccess ? AcceptingNameStore(session: store) : nil
    }

    private static func nameDraft(for scenario: ProfileSampleScenario) -> String? {
        switch scenario {
        // Over the 32-character limit: the field shows the broken rule and Save stays off.
        case .validation: return "Logan Norman of the Weekend Investors"
        // Valid and different from the saved name, so Save is live and can be rejected.
        case .saveFailure: return "Logan N"
        // Valid and different, and this scenario's store accepts it.
        case .saveSuccess: return "Logan N"
        default: return nil
        }
    }

    private static func fill(_ session: AppSessionStore, for scenario: ProfileSampleScenario) {
        session.isLoading = false
        switch scenario {
        case .loading:
            session.isLoading = true
            return
        case .error:
            session.errorMessage = "Could not connect to Monaco."
            return
        default:
            break
        }

        session.profile = sampleProfile(
            userID: "sample-user",
            displayName: "Logan Norman",
            photoURL: scenario == .photo || scenario == .cabals ? samplePhotoURL() : nil,
            createdAt: ISO8601DateFormatter().date(from: "2026-09-01T14:30:00Z")
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

    static func samplePhotoURL() -> URL? {
        SampleImage.flat(.plum, name: "avatar")
    }
}

/// A store that accepts the save, standing in for the profile endpoint. Writes the name back
/// to the session, so the screen behind the sheet really does show it afterwards.
private struct AcceptingNameStore: DisplayNameSaving {
    let session: AppSessionStore

    func saveDisplayName(_ draft: String) async -> ProfileSaveOutcome {
        guard case .success(let normalized) = DisplayNameRules.normalize(draft) else {
            return .failed("That name can't be used.")
        }
        guard let current = session.profile else { return .failed("Your profile is still loading.") }
        guard normalized != current.displayName else { return .unchanged }
        session.profile = current.withDisplayName(normalized)
        return .saved
    }
}

final class ProfileSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ProfileSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(SampleAPIScript(mode: scenario == .empty ? .empty : .populated))
        return AnyView(ProfileSampleHarness(scenario: scenario, auth: auth))
    }
}
#endif
