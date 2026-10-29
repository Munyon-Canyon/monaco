import MonacoAnalytics
import MonacoCore
import UserNotifications

extension PushCategory {
    var notificationCategory: UNNotificationCategory {
        UNNotificationCategory(
            identifier: id,
            actions: actions.map(\.notificationAction),
            intentIdentifiers: [],
            options: []
        )
    }
}

extension PushActionSpec {
    var notificationAction: UNNotificationAction {
        var options: UNNotificationActionOptions = []
        if foreground { options.insert(.foreground) }
        if authenticationRequired { options.insert(.authenticationRequired) }
        return UNNotificationAction(
            identifier: id,
            title: title,
            options: options,
            icon: UNNotificationActionIcon(systemImageName: symbol)
        )
    }
}

final class PushCenterDelegate: NSObject, @preconcurrency UNUserNotificationCenterDelegate {
    private let router: PushRouter

    init(router: PushRouter) {
        self.router = router
    }

    @MainActor
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        let userInfo = notification.request.content.userInfo
        let shown = (userInfo["proposal_id"] as? String).flatMap { router.isShowing(proposalID: $0) ? $0 : nil }
        return Self.presentation(for: userInfo, shownProposalID: shown)
    }

    static func presentation(
        for userInfo: [AnyHashable: Any],
        shownProposalID: String? = nil
    ) -> UNNotificationPresentationOptions {
        if userInfo["kind"] as? String == "deposit_credited" { return [] }
        if let shownProposalID, userInfo["proposal_id"] as? String == shownProposalID { return [.list] }
        return [.banner, .list, .sound]
    }

    @MainActor
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse
    ) async {
        let userInfo = response.notification.request.content.userInfo
        let payload = Dictionary(
            userInfo.compactMap { key, value in (key as? String).map { ($0, value) } },
            uniquingKeysWith: { first, _ in first }
        )
        AppAnalytics.current.step(
            .vote(.pushAction),
            properties: [
                "action": .string(PushAction.analyticsName(actionIdentifier: response.actionIdentifier)),
                "kind": .string(payload["kind"] as? String ?? ""),
            ]
        )
        router.handle(PushAction.route(actionIdentifier: response.actionIdentifier, payload: payload))
    }
}
