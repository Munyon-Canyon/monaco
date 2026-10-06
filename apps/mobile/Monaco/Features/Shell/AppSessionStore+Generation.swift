import MonacoAPI
import MonacoCore

extension AppSessionStore {
    func failOpen(_ error: Error, rejectedToken: String, auth: SessionAuthenticating) async {
        isLoading = false
        if case APIError.accountDeleted = error {
            await auth.signOut(reason: ToastCopy.message(for: .accountDeleted), rejectedToken: rejectedToken)
            return
        }
        if case APIError.signedOut = error {
            await auth.signOut(reason: LoginFailureCopy.sessionExpired, rejectedToken: rejectedToken)
            return
        }
        if case APIError.missingAccessToken = error {
            errorMessage = ToastCopy.message(for: .missingAccessToken(""))
            return
        }
        let mapped = SessionErrorMapping.describe(error, apiBaseURL: Config.apiBaseURL)
        errorMessage = mapped.message
        #if DEBUG
        errorDebugDetail = "\(mapped.debugDetail)\n\(Config.api.debugSummary)"
        #endif
    }

    func failedProfileLoad(_ error: Error, token: String, auth: SessionAuthenticating) async -> SessionProfile? {
        if case APIError.accountDeleted = error {
            await auth.signOut(reason: ToastCopy.message(for: .accountDeleted), rejectedToken: token)
        } else if case APIError.signedOut = error {
            await auth.signOut(reason: LoginFailureCopy.sessionExpired, rejectedToken: token)
        } else if case APIError.missingAccessToken = error {
            errorMessage = ToastCopy.message(for: .missingAccessToken(""))
        }
        return nil
    }

    func resolvedAccessToken(_ provided: String?, auth: SessionAuthenticating) async -> String? {
        if let provided { return provided }
        return await accessToken(auth: auth)
    }

    func accessToken(auth: SessionAuthenticating) async -> String? {
        return auth.accessToken
    }
    func noteProfileWrite() {
        bumpProfileWriteGeneration()
    }

    func mayWrite(_ generation: Int) -> Bool {
        generation == refreshGenerationValue() && !Task.isCancelled
    }

    func finishRefresh(
        profile: SessionProfile?,
        generation: Int,
        profileGeneration: Int,
        auth: SessionAuthenticating,
        token: String
    ) async {
        guard mayWrite(generation), await accessToken(auth: auth) == token else { return }
        hasLoaded = true
        if let profile, profileGeneration == profileWriteGenerationValue(), mayWrite(generation) {
            self.profile = profile
        }
        guard mayWrite(generation) else { return }
        errorMessage = nil
    }
}
