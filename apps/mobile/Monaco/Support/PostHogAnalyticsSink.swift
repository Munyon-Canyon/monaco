import MonacoAnalytics
import MonacoCore
import PostHog

struct PostHogAnalyticsSink: AnalyticsSink {
    static let host = "https://us.i.posthog.com"

    static func superProperties(for environment: MonacoEnvironment) -> [String: Any] {
        ["env": environment.rawValue]
    }

    init(apiKey: String, environment: MonacoEnvironment) {
        let config = PostHogConfig(projectToken: apiKey, host: Self.host)
        config.captureApplicationLifecycleEvents = true
        config.captureScreenViews = false
        config.captureElementInteractions = false
        config.sessionReplay = false
        config.personProfiles = .identifiedOnly
        PostHogSDK.shared.setup(config)
        PostHogSDK.shared.register(Self.superProperties(for: environment))
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
