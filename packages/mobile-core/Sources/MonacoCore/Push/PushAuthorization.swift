public enum PushAuthorization: Sendable, CaseIterable {
    case notDetermined
    case denied
    case authorized

    public init(_ status: PushAuthorizationStatus) {
        switch status {
        case .notDetermined: self = .notDetermined
        case .denied: self = .denied
        case .authorized, .provisional, .ephemeral: self = .authorized
        }
    }
}

public protocol NotificationAuthorizing: Sendable {
    func status() async -> PushAuthorization
    func request() async -> Bool
}

public enum PushPrePromptPolicy {
    public static func shouldShow(status: PushAuthorization, alreadyShown: Bool) -> Bool {
        switch status {
        case .notDetermined: !alreadyShown
        case .denied, .authorized: false
        }
    }
}
