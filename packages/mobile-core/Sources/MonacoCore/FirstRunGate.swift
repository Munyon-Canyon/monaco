import Foundation

public enum FirstRunDestination: Equatable, Sendable {
    case session
    case nameSetup
    case app
}

/// The post-sign-in routing decision, kept pure so it can be tested without a simulator
/// and can never disagree with the screen it drives.
public enum FirstRunGate {
    /// A name that is empty once trimmed is the same as no name at all: every social
    /// surface (proposals, votes, leaderboards, chat) renders those accounts as "Member".
    ///
    /// Whitespace-only names cannot be saved — `DisplayNameRules.normalize` rejects them
    /// and so does the backend — so they only arrive from an older client or a direct
    /// database write. Treat them as missing rather than letting them through the gate.
    public static func needsDisplayName(_ displayName: String) -> Bool {
        displayName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    public static func destination(for profile: SessionProfile?) -> FirstRunDestination {
        guard let profile else { return .session }
        return needsDisplayName(profile.displayName) ? .nameSetup : .app
    }
}
