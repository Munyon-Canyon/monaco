import MonacoCore
import UserNotifications
import os

@MainActor
struct LiveNotificationAuthorizing: NotificationAuthorizing {
    func status() async -> PushAuthorization {
        let settings = await UNUserNotificationCenter.current().notificationSettings()
        return PushAuthorization(PushAuthorizationStatus(settings.authorizationStatus))
    }

    func request() async -> Bool {
        do {
            return try await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound])
        } catch {
            AppLogger.session.error(
                "Notification permission request failed: \(String(describing: error), privacy: .public)")
            return false
        }
    }
}
