import Foundation

public enum LinkError: Error, Equatable, Sendable {
    case alreadyLinkedElsewhere
    case alreadyHasPhone
    case invalidCode
    case cancelled
    case network
    case unknown
    case unavailable
}

public protocol AccountLinking: Sendable {
    func sendPhoneCode(e164: String) async throws
    func linkPhone(code: String) async throws
    func linkX() async throws
}

public enum LogRedaction {
    public static let phoneMarker = "<phone>"

    public static func phoneNumbers(in text: String) -> String {
        text.replacing(phoneRun, with: { _ in phoneMarker })
    }

    private static var phoneRun: Regex<Substring> { /\+?\d(?:[\d\s().\-]*\d){6,}/ }
}
