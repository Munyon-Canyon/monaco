import Foundation
import MonacoCore

/// Result of a profile write, phrased for a toast.
enum ProfileSaveOutcome: Equatable {
    case saved
    case unchanged
    case failed(String)
}

/// Self-profile writes. After a save, `profile` holds the server copy and home boards are
/// refetched so the new name and photo show up on the people and member boards.
extension AppSessionStore {
    /// Optimistically renames the signed-in user, rolling back if the server rejects it.
    ///
    /// First run passes `optimistic: false`. `FirstRunGate` routes on `profile.displayName`,
    /// so writing the name before the server confirms it would drop the user into the
    /// tabs mid-request and bounce them back out on a rejection.
    func updateDisplayName(
        _ draft: String,
        auth: SessionAuthenticating,
        optimistic: Bool = true
    ) async -> ProfileSaveOutcome {
        guard let current = profile else {
            return .failed("Your profile is still loading.")
        }
        let normalized: String
        switch DisplayNameRules.normalize(draft) {
        case .success(let value):
            normalized = value
        case .failure(let error):
            return .failed(error.message)
        }
        guard normalized != current.displayName else {
            return .unchanged
        }
        return .failed("Could not save your name. Try again.")
    }

    /// Uploads an already-prepared photo (see `ProfilePhotoUploadPreparer`).
    func uploadProfilePhoto(
        _ imageData: Data,
        mimeType: String,
        auth: SessionAuthenticating
    ) async -> ProfileSaveOutcome {
        guard let (client, token) = await profileClient(auth: auth) else {
            return .failed("Sign in again to change your photo.")
        }
        let generation = refreshGenerationValue()
        let writeGeneration = profileWriteGenerationValue()
        do {
            let saved = try await client.uploadProfilePhoto(imageData: imageData, mimeType: mimeType)
            guard mayWrite(generation), writeGeneration == profileWriteGenerationValue() else {
                return .failed("Sign in again to change your photo.")
            }
            noteProfileWrite()
            if let current = profile {
                profile = current.replacing(from: saved)
            }
        } catch {
            return await failure(
                for: error,
                auth: auth,
                rejectedToken: token,
                fallback: "Could not upload your photo. Try again."
            )
        }
        refreshBoardsAfterProfileWrite(auth: auth)
        return .saved
    }

    /// The client for this write plus the token it runs under, so a 401 can be reported
    /// against the token that was actually rejected.
    private func profileClient(auth: SessionAuthenticating) async -> (MonacoCore.MonacoAPIClient, String)? {
        guard let token = await accessToken(auth: auth), !token.isEmpty else { return nil }
        let client = profileClientFactory(token)
        return (client, token)
    }

    private func failure(
        for error: Error,
        auth: SessionAuthenticating,
        rejectedToken: String,
        fallback: String
    ) async -> ProfileSaveOutcome {
        if case MonacoCore.MonacoAPIError.httpStatus(401, _) = error {
            await auth.signOutAfterRejectedSession(rejectedToken: rejectedToken)
            return .failed(LoginFailureCopy.sessionExpired)
        }
        return .failed(Self.profileErrorMessage(for: error, fallback: fallback))
    }

    static func profileErrorMessage(for error: Error, fallback: String) -> String {
        switch error {
        case MonacoCore.MonacoAPIError.rejected(_, let message, _):
            return message
        case MonacoCore.MonacoAPIError.rateLimited(let retryAfter, _):
            if let retryAfter, retryAfter > 0 {
                return "Too many changes. Try again in \(retryAfter)s."
            }
            return "Too many changes. Try again in a minute."
        case MonacoCore.MonacoAPIError.httpStatus(503, _):
            return "Photo uploads are not set up on this server."
        case let urlError as URLError where urlError.code != .cancelled:
            return "Could not reach Monaco. Check your connection."
        default:
            return fallback
        }
    }
}
