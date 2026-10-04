import Foundation
import MonacoAPI
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
    func updateDisplayName(
        _ draft: String,
        auth: SessionAuthenticating,
        optimistic: Bool = true,
        submission: IdempotentSubmission
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
        guard let sessions else {
            return .failed("Could not save your name. Try again.")
        }
        guard let token = await accessToken(auth: auth) else {
            return .failed("Sign in again to edit your profile.")
        }
        noteProfileWrite()
        let writeGeneration = profileWriteGenerationValue()
        let pending = current.withDisplayName(normalized)
        if optimistic {
            profile = pending
        }
        do {
            let saved = try await sessions.updateDisplayName(normalized, submission: submission)
            guard writeGeneration == profileWriteGenerationValue() else {
                return .failed("Sign in again to edit your profile.")
            }
            profile = saved
            noteProfileWrite()
        } catch {
            if profile == pending {
                profile = current
            }
            return await failure(for: error, auth: auth, rejectedToken: token)
        }
        refreshBoardsAfterProfileWrite(auth: auth)
        return .saved
    }

    func saveProfilePhoto(
        _ photo: Data,
        auth: SessionAuthenticating,
        submission: IdempotentSubmission
    ) async -> ProfileSaveOutcome {
        guard let sessions, let token = await accessToken(auth: auth), !token.isEmpty else {
            return .failed("Sign in again to change your photo.")
        }
        noteProfileWrite()
        let writeGeneration = profileWriteGenerationValue()
        do {
            let saved = try await sessions.uploadProfilePhoto(photo, submission: submission)
            guard writeGeneration == profileWriteGenerationValue() else {
                return .failed("Sign in again to change your photo.")
            }
            profile = saved
            noteProfileWrite()
        } catch {
            return await failure(for: error, auth: auth, rejectedToken: token)
        }
        refreshBoardsAfterProfileWrite(auth: auth)
        return .saved
    }

    private func failure(
        for error: Error,
        auth: SessionAuthenticating,
        rejectedToken: String
    ) async -> ProfileSaveOutcome {
        let apiError = APIError(error)
        if case .accountDeleted = apiError {
            await auth.signOut(reason: ToastCopy.message(for: .accountDeleted), rejectedToken: rejectedToken)
        }
        return .failed(ProfileSaveFailure(apiError).message)
    }
}
