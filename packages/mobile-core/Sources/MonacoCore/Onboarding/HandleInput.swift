import Foundation
import MonacoAPI

public enum HandleReason: Equatable, Sendable {
    case taken
    case reserved
    case invalid
    case tooSoon

    public func message(changeableAt: Date?, timeZone: TimeZone = .current) -> String {
        switch self {
        case .taken: HandleCopy.taken
        case .reserved: HandleCopy.reserved
        case .invalid: HandleCopy.invalid
        case .tooSoon:
            changeableAt.map { HandleCopy.tooSoon(on: HandleInput.dateLabel($0, timeZone: timeZone)) }
                ?? HandleCopy.tooSoonUndated
        }
    }
}

public enum HandleStatus: Equatable, Sendable {
    case idle
    case checking(String)
    case available(String)
    case unavailable(String, HandleReason)
    case slowDown(String)
    case failed(String)

    public var claimable: String? {
        if case .available(let handle) = self { handle } else { nil }
    }
}

public enum HandleStepReason: Equatable, Sendable {
    case firstRun
    case revoked

    public var subtext: String {
        switch self {
        case .firstRun: HandleCopy.firstRunSubtext
        case .revoked: HandleCopy.revokedSubtext
        }
    }
}

public enum HandleSaveFailure: Equatable, Sendable {
    case inline(HandleReason)
    case toast(String)

    public init(_ error: APIError) {
        if case .problem(let problem) = error, case .known(let code) = problem.code,
            let reason = Self.reasons[code]
        {
            self = .inline(reason)
        } else {
            self = .toast(ToastCopy.message(for: error))
        }
    }

    private static let reasons: [Components.Schemas.ErrorCode: HandleReason] = [
        .handleTaken: .taken, .handleReserved: .reserved, .handleInvalid: .invalid, .handleTooSoon: .tooSoon,
    ]
}

public enum HandleInput {
    public static let lengths = 3...20

    public static func normalize(_ raw: String) -> String {
        var handle = raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if handle.hasPrefix("@") { handle.removeFirst() }
        return handle
    }

    public static func localReason(_ handle: String) -> HandleReason? {
        let scalars = handle.unicodeScalars
        guard lengths.contains(scalars.count), scalars.allSatisfy(isHandleScalar) else { return .invalid }
        return nil
    }

    public static func stepReason(for profile: SessionProfile) -> HandleStepReason {
        profile.authState == .created ? .firstRun : .revoked
    }

    static func dateLabel(_ date: Date, timeZone: TimeZone) -> String {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = timeZone
        formatter.dateFormat = "MMMM d, yyyy"
        return formatter.string(from: date)
    }

    private static func isHandleScalar(_ scalar: Unicode.Scalar) -> Bool {
        ("a"..."z").contains(scalar) || ("0"..."9").contains(scalar) || scalar == "_"
    }
}

public enum HandleCopy {
    public static let firstRunSubtext = "This is how people find you on Monaco."
    public static let revokedSubtext = "Your handle was removed. Pick a new one."
    public static let taken = "That handle is taken."
    public static let reserved = "That handle isn't available."
    public static let invalid = "Use 3 to 20 letters, numbers or underscores."
    public static let tooSoonUndated = "You can't change your handle yet."
    public static let checking = "Checking…"
    public static let available = "Available"
    public static let slowDown = "Slow down a little."
    public static let checkFailed = "Couldn't check that handle."
    public static let tryAgain = "Try again"
    public static let saving = "Saving…"
    public static let updated = "Handle updated."

    public static func tooSoon(on date: String) -> String {
        "You can change your handle again on \(date)."
    }
}
