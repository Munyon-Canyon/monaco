import MonacoCore
import UserNotifications

final class PushCenterDelegate: NSObject, @preconcurrency UNUserNotificationCenterDelegate {
    private let router: PushRouter

    init(router: PushRouter) {
        self.router = router
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        Self.presentation(for: notification.request.content.userInfo)
    }

    static func presentation(for userInfo: [AnyHashable: Any]) -> UNNotificationPresentationOptions {
        userInfo["kind"] as? String == "deposit_credited" ? [] : [.banner, .list, .sound]
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse
    ) async {
        let userInfo = response.notification.request.content.userInfo
        let payload = Dictionary(
            userInfo.compactMap { key, value in (key as? String).map { ($0, value) } },
            uniquingKeysWith: { first, _ in first }
        )
        router.handle(PushRoute.parse(payload))
    }
}
