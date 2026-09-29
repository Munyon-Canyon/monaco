import Foundation

/// The two ways into the app. Apple is required by App Store guideline 4.8 once Google is offered.
public enum LoginProvider: Equatable, Sendable {
    case apple
    case google
}

/// Where sign-in is: `idle → authorizing(provider) → authenticated | failed`, plus the launch-time
/// restore of a saved session. Pure, so every transition is host-testable without Privy.
public enum LoginPhase: Equatable, Sendable {
    /// Launch: a previous sign-in exists and Privy is restoring it. The gate shows a splash,
    /// not the login screen, until this resolves.
    case restoring
    /// The previous sign-in could not be checked right now (offline). Retryable; the user stays
    /// signed in.
    case restoreFailed(message: String)
    case idle
    /// The Apple or Google sheet is up.
    case authorizing(LoginProvider)
    case failed(message: String)
    case authenticated(userID: String)

    public var isBusy: Bool {
        if case .authorizing = self { return true }
        return false
    }

    /// What the login screen toasts. Nil in every phase but a failure, so a cancelled sheet,
    /// which lands on `idle`, says nothing.
    public var toastMessage: String? {
        if case .failed(let message) = self { return message }
        return nil
    }

    /// Returns false while a sheet is already up or a session exists, so a second tap cannot
    /// open a second sheet.
    public mutating func beginAuthorizing(_ provider: LoginProvider) -> Bool {
        switch self {
        case .idle, .failed:
            self = .authorizing(provider)
            return true
        case .restoring, .restoreFailed, .authorizing, .authenticated:
            return false
        }
    }

    /// The member closing the sheet is not a failure: back to the buttons, with no toast.
    public mutating func authorizationFailed(_ failure: LoginFailure) {
        if failure == .cancelled {
            self = .idle
        } else {
            self = .failed(message: LoginFailureCopy.message(for: failure, step: .authorize))
        }
    }
}
