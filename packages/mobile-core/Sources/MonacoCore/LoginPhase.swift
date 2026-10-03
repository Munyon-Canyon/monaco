import Foundation

public enum LoginProvider: String, Equatable, Sendable {
    case apple
    case google
}

public enum LoginPhase: Equatable, Sendable {
    case restoring
    case restoreFailed(message: String)
    case idle
    case sendingCode
    case awaitingCode
    case verifyingCode
    case authorizing(LoginProvider)
    case failed(message: String)
    case providerFailed(LoginProvider, message: String)
    case authenticated(userID: String)
}

public struct LoginFlow: Equatable, Sendable {
    public enum Step: Equatable, Sendable {
        case enterAddress
        case enterCode(destination: String)

        public var isCodeEntry: Bool {
            if case .enterCode = self { return true }
            return false
        }

        public var destination: String? {
            if case .enterCode(let destination) = self { return destination }
            return nil
        }
    }

    public private(set) var step: Step = .enterAddress
    public private(set) var phase: LoginPhase

    public init(phase: LoginPhase = .idle) {
        self.phase = phase
    }

    public var isBusy: Bool {
        switch phase {
        case .sendingCode, .verifyingCode, .authorizing: return true
        default: return false
        }
    }

    public var isCodeEntry: Bool { step.isCodeEntry }
    public var destination: String? { step.destination }

    public var toastMessage: String? {
        if case .providerFailed(_, let message) = phase { return message }
        return nil
    }

    public mutating func beginSend() -> Bool {
        guard !phase.isAuthenticated else { return false }
        guard !isBusy else { return false }
        phase = .sendingCode
        return true
    }

    public mutating func sendSucceeded(destination: String) {
        step = .enterCode(destination: destination)
        phase = .awaitingCode
    }

    public mutating func sendFailed(message: String) {
        phase = .failed(message: message)
    }

    public mutating func beginVerify() -> Bool {
        guard !phase.isAuthenticated else { return false }
        guard !isBusy else { return false }
        phase = .verifyingCode
        return true
    }

    public mutating func verifyFailed(message: String) {
        phase = .failed(message: message)
    }

    public mutating func beginAuthorizing(_ provider: LoginProvider) -> Bool {
        guard !phase.isAuthenticated else { return false }
        switch phase {
        case .idle, .awaitingCode, .failed, .providerFailed:
            phase = .authorizing(provider)
            return true
        case .restoring, .restoreFailed, .sendingCode, .verifyingCode, .authorizing, .authenticated:
            return false
        }
    }

    public mutating func authorizationFailed(_ failure: LoginFailure) {
        guard case .authorizing(let provider) = phase else { return }
        if failure == .cancelled {
            phase = .idle
        } else {
            phase = .providerFailed(provider, message: LoginFailureCopy.message(for: failure, step: .authorize))
        }
    }

    public mutating func authenticated(userID: String) {
        step = .enterAddress
        phase = .authenticated(userID: userID)
    }

    public mutating func returnToAddressEntry() {
        switch phase {
        case .authenticated, .restoring, .restoreFailed, .authorizing:
            return
        default:
            step = .enterAddress
            phase = .idle
        }
    }

    public mutating func signedOut() {
        step = .enterAddress
        phase = .idle
    }

    public mutating func restoring() {
        step = .enterAddress
        phase = .restoring
    }

    public mutating func restoreFailed(message: String) {
        phase = .restoreFailed(message: message)
    }

    public mutating func tokenUnavailable(message: String) {
        if case .authorizing(let provider) = phase {
            phase = .providerFailed(provider, message: message)
        } else {
            phase = .failed(message: message)
        }
    }
}

extension LoginPhase {
    fileprivate var isAuthenticated: Bool {
        if case .authenticated = self { return true }
        return false
    }
}
