import Foundation

public enum PushDeviceToken {
    public static func hex(_ data: Data) -> String {
        data.map { String(format: "%02x", $0) }.joined()
    }
}

public enum PushEnvironment: String, Sendable, CaseIterable {
    case sandbox
    case production

    public init?(infoValue: String?) {
        guard let infoValue, let environment = Self(rawValue: infoValue) else { return nil }
        self = environment
    }
}

public enum PushAuthorizationStatus: Sendable, CaseIterable {
    case notDetermined
    case denied
    case authorized
    case provisional
    case ephemeral
}

public enum PushRegistrationPolicy {
    public static func shouldRegister(status: PushAuthorizationStatus, isSignedIn: Bool) -> Bool {
        switch status {
        case .authorized: isSignedIn
        case .notDetermined, .denied, .provisional, .ephemeral: false
        }
    }
}
