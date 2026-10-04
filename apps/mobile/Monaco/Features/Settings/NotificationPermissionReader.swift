import UserNotifications

protocol NotificationPermissionReading: Sendable {
    func isAuthorized() async -> Bool
}

struct LiveNotificationPermissionReader: NotificationPermissionReading {
    func isAuthorized() async -> Bool {
        switch await UNUserNotificationCenter.current().notificationSettings().authorizationStatus {
        case .authorized, .provisional, .ephemeral: true
        case .denied, .notDetermined: false
        @unknown default: false
        }
    }
}

struct FixedNotificationPermissionReader: NotificationPermissionReading {
    let authorized: Bool

    func isAuthorized() async -> Bool { authorized }
}
