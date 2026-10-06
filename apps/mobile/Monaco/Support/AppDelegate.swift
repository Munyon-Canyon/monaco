import MonacoCore
import UIKit
import UserNotifications
import os

final class AppDelegate: NSObject, UIApplicationDelegate {
    static weak var environment: AppEnvironment?

    private var pushCenter: PushCenterDelegate?

    static func registerIfAuthorized() async {
        let settings = await UNUserNotificationCenter.current().notificationSettings()
        guard let environment else { return }
        let status = PushAuthorizationStatus(settings.authorizationStatus)
        guard PushRegistrationPolicy.shouldRegister(status: status, isSignedIn: environment.hasOpenSession) else {
            return
        }
        UIApplication.shared.registerForRemoteNotifications()
    }

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        guard let environment = Self.environment else {
            AppLogger.session.error("Push taps will not open a screen: the app environment is missing at launch")
            return true
        }
        let center = PushCenterDelegate(router: PushRouter(environment: environment))
        pushCenter = center
        UNUserNotificationCenter.current().delegate = center
        return true
    }

    func application(
        _ application: UIApplication,
        didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data
    ) {
        guard let environment = Self.environment else { return }
        let token = PushDeviceToken.hex(deviceToken)
        Task {
            do {
                try await environment.push.register(token: token)
            } catch {
                AppLogger.session.error("Push registration failed: \(String(describing: error), privacy: .public)")
            }
        }
    }

    func application(
        _ application: UIApplication,
        didFailToRegisterForRemoteNotificationsWithError error: any Error
    ) {
        AppLogger.session.error(
            "Remote notification registration failed: \(String(describing: error), privacy: .public)")
    }
}

extension PushAuthorizationStatus {
    init(_ status: UNAuthorizationStatus) {
        switch status {
        case .notDetermined: self = .notDetermined
        case .denied: self = .denied
        case .authorized: self = .authorized
        case .provisional: self = .provisional
        case .ephemeral: self = .ephemeral
        @unknown default: self = .denied
        }
    }
}
