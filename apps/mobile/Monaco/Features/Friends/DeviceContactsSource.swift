import Contacts
import MonacoCore

@MainActor
final class DeviceContactsSource: ContactsSource {
    private let store = CNContactStore()

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
            let granted = try await store.requestAccess(for: .contacts)
            return granted ? .granted : currentAccess()
        } catch {
            return .denied
        }
    }

    func phoneNumbers() throws -> [String] {
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
