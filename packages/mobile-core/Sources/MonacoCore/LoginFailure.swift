import Foundation

/// Why a one-time-code step failed, reduced to what the user can act on.
public enum LoginFailure: Equatable, Sendable {
    case cancelled
    /// Wrong or expired code.
    case codeRejected
    /// The device could not reach the sign-in provider.
    case offline
    /// Too many attempts; the provider is throttling this phone or email.
    case rateLimited
    case signUpsPaused
    case methodUnavailable
    /// Anything else. `detail` is provider copy, when it said something useful.
    case other(detail: String?)

    /// Whether the code field should stay on screen so the user can try again
    /// without requesting a new code.
    public var keepsCodeEntry: Bool {
        switch self {
        case .codeRejected, .offline: return true
        case .cancelled, .rateLimited, .signUpsPaused, .methodUnavailable, .other: return false
        }
    }
}

public enum LoginStep: Equatable, Sendable {
    case authorize
    case sendCode
    case verifyCode
}

public enum LoginFailureCopy {
    public static let sessionExpired = "Your session expired. Sign in again."
    public static let restoreOffline = "Can't reach the sign-in service. Check your connection and try again."
    public static let tokenUnavailable = "Signed in, but couldn't finish. Check your connection and try again."

    public static func message(for failure: LoginFailure, step: LoginStep) -> String {
        switch (failure, step) {
        case (.cancelled, _):
            return "Sign-in cancelled."
        case (.offline, _):
            return "No connection. Check your internet and try again."
        case (.rateLimited, _):
            return "Too many attempts. Wait a minute, then try again."
        case (.signUpsPaused, _):
            return "Sign-ups are paused right now. Try again later."
        case (.methodUnavailable, _):
            return "This sign-in method isn't available."
        case (.codeRejected, _):
            return "That code didn't work. Check it, or send a new one."
        case (.other(let detail), .sendCode):
            return prefixing(detail, to: "Couldn't send the code. Try again.")
        case (.other, .authorize):
            return "Couldn't sign you in. Try again."
        case (.other(let detail), .verifyCode):
            return prefixing(detail, to: "Couldn't sign you in. Try again.")
        }
    }

    private static let quotaCode = "max_accounts_reached"
    private static let methodCode = "disallowed_login_method"
    private static let wrongCodeCode = "invalid_credentials"

    public static func failure(
        forHTTPStatus status: Int, step: LoginStep, detail: String?, errorCode: String? = nil
    ) -> LoginFailure {
        switch errorCode {
        case quotaCode: return .signUpsPaused
        case methodCode: return .methodUnavailable
        default: break
        }
        switch status {
        case 429:
            return .rateLimited
        case 400, 401, 403, 404, 422:
            guard step == .verifyCode else { return .other(detail: detail) }
            if let errorCode, errorCode != wrongCodeCode { return .other(detail: detail) }
            return .codeRejected
        default:
            return .other(detail: detail)
        }
    }

    private static func prefixing(_ detail: String?, to advice: String) -> String {
        let trimmed = detail?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !trimmed.isEmpty else { return advice }
        let endsSentence = trimmed.last.map { ".!?".contains($0) } ?? false
        let reason = endsSentence ? trimmed : "\(trimmed)."
        return "\(reason) Try again."
    }
}
