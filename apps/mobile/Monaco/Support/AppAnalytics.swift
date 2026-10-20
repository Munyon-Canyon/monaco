import Foundation
import MonacoAnalytics
import MonacoCore

@MainActor
enum AppAnalytics {
    static var current = Analytics(sink: NoopAnalyticsSink())

    static func makeDefault(apiKey: String = Config.postHogAPIKey) -> Analytics {
        Analytics(
            sink: makeSink(
                apiKey: apiKey, isTest: Config.isRunningTests, environment: Config.api.environment,
                replayEnabled: Config.postHogSessionReplay))
    }

    static func makeSink(
        apiKey: String, isTest: Bool, environment: MonacoEnvironment, replayEnabled: Bool = false
    ) -> any AnalyticsSink {
        let logging = LoggingAnalyticsSink()
        guard !apiKey.isEmpty, !isTest else { return logging }
        let postHog = PostHogAnalyticsSink(apiKey: apiKey, environment: environment, replayEnabled: replayEnabled)
        guard MonacoBuildKind.current == .debug else { return postHog }
        return CompositeAnalyticsSink(sinks: [postHog, logging])
    }

    static func capture(_ event: ReferralAppEvent) {
        let step: AnalyticsStep
        switch event {
        case .invitePasteShown: step = .referral(.invitePasteShown)
        case .invitePasted: step = .referral(.invitePasted)
        case .inviteSkipped: step = .referral(.inviteSkipped)
        }
        if event == .invitePasteShown { current.start(.referral) }
        current.step(step)
    }
}
