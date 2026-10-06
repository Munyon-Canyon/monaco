import Foundation

public enum ContactsAccess: Equatable, Sendable {
    case notDetermined
    case granted
    case denied
}

@MainActor
public protocol ContactsSource: AnyObject {
    func currentAccess() -> ContactsAccess
    func requestAccess() async -> ContactsAccess
    func phoneNumbers() throws -> [String]
}
