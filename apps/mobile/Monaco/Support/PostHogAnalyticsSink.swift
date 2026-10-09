import MonacoAnalytics
import PostHog

struct PostHogAnalyticsSink: AnalyticsSink {
    static let host = "https://us.i.posthog.com"

    init(apiKey: String) {
        let config = PostHogConfig(projectToken: apiKey, host: Self.host)
        config.captureApplicationLifecycleEvents = true
        config.captureScreenViews = false
        config.captureElementInteractions = false
        config.sessionReplay = false
        config.personProfiles = .identifiedOnly
        PostHogSDK.shared.setup(config)
    }

    func identify(userID: String, properties: [String: AnalyticsValue]) {
        PostHogSDK.shared.identify(userID, userProperties: properties.mapValues(\.foundationValue))
    }

    func capture(_ name: String, properties: [String: AnalyticsValue]) {
        PostHogSDK.shared.capture(name, properties: properties.mapValues(\.foundationValue))
    }

    func screen(_ name: String) {
        PostHogSDK.shared.screen(name)
    }

    func reset() {
        PostHogSDK.shared.reset()
    }
}

extension AnalyticsValue {
    fileprivate nonisolated var foundationValue: Any {
        switch self {
        case .string(let value): value
        case .int(let value): value
        case .double(let value): value
        case .bool(let value): value
        }
    }
}
