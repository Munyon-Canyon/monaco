import Contacts
import MonacoCore
import os

@MainActor
final class DeviceContactsSource: ContactsSource {
    private static let log = Logger(subsystem: "so.monaco.app", category: "contacts")

    func currentAccess() -> ContactsAccess {
        switch CNContactStore.authorizationStatus(for: .contacts) {
        case .authorized, .limited:
            return .granted
        case .denied, .restricted:
            return .denied
        case .notDetermined:
            return .notDetermined
        @unknown default:
            return .notDetermined
        }
    }

    func requestAccess() async -> ContactsAccess {
        do {
            let granted = try await CNContactStore().requestAccess(for: .contacts)
            return granted ? .granted : currentAccess()
        } catch {
            return .denied
        }
    }

    func phoneNumbers() async throws -> [String] {
        do {
            return try await Task.detached { try Self.readNumbers() }.value
        } catch {
            let failure = error as NSError
            Self.log.error(
                "contacts read failed: \(failure.domain, privacy: .public) \(failure.code, privacy: .public)")
            throw error
        }
    }

    private nonisolated static func readNumbers() throws -> [String] {
        let store = CNContactStore()
        let request = CNContactFetchRequest(keysToFetch: [CNContactPhoneNumbersKey as CNKeyDescriptor])
        var numbers: [String] = []
        try store.enumerateContacts(with: request) { contact, _ in
            for number in contact.phoneNumbers {
                numbers.append(number.value.stringValue)
            }
        }
        return numbers
    }
}
