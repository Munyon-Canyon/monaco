#if DEBUG
import MonacoCore
import UserNotifications

@MainActor
enum SampleVotePush {
    static let argument = "-MonacoSampleVotePush"
    static let cabalID = "01920000-0000-7000-8000-000000000c01"
    private static var center: PushCenterDelegate?

    static func scheduleIfRequested(arguments: [String] = ProcessInfo.processInfo.arguments) {
        guard let flag = arguments.firstIndex(of: argument), arguments.indices.contains(flag + 1) else { return }
        let proposalID = arguments[flag + 1]
        Task {
            await installDelegateWhenEnvironmentExists()
            await schedule(proposalID: proposalID)
        }
    }

    private static func installDelegateWhenEnvironmentExists() async {
        while true {
            if let environment = AppDelegate.environment, environment.viewer != nil {
                let delegate = PushCenterDelegate(router: PushRouter(environment: environment))
                center = delegate
                UNUserNotificationCenter.current().delegate = delegate
                return
            }
            try? await Task.sleep(for: .milliseconds(100))
        }
    }

    private static func schedule(proposalID: String) async {
        let notifications = UNUserNotificationCenter.current()
        guard (try? await notifications.requestAuthorization(options: [.alert, .sound])) == true else { return }
        let content = UNMutableNotificationContent()
        content.title = "3 votes left · ends in 4h"
        content.body = "Buy NVDA $400: 4 yes, 1 no so far. Yours counts."
        content.categoryIdentifier = "VOTE"
        content.threadIdentifier = "cabal-\(cabalID)"
        content.sound = .default
        content.interruptionLevel = .timeSensitive
        content.userInfo = [
            "kind": "vote_reminder",
            "cabal_id": cabalID,
            "proposal_id": proposalID,
            "sender_kind": "cabal",
            "sender_id": cabalID,
            "sender_name": "AC Capital",
            "group_id": cabalID,
            "group_name": "AC Capital",
        ]
        let request = UNNotificationRequest(
            identifier: "sample-vote-push",
            content: content,
            trigger: UNTimeIntervalNotificationTrigger(timeInterval: 2, repeats: false)
        )
        try? await notifications.add(request)
    }
}
#endif
