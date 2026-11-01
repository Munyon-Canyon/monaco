import MonacoAnalytics
import MonacoCore
import PostHog

struct PostHogAnalyticsSink: AnalyticsSink {
    static let host = "https://us.i.posthog.com"

    static func superProperties(for environment: MonacoEnvironment) -> [String: Any] {
        ["env": environment.rawValue]
    }

    static func recordsSessions(environment: MonacoEnvironment, replayEnabled: Bool) -> Bool {
        replayEnabled && environment == .staging
    }

    static func makeConfig(apiKey: String, environment: MonacoEnvironment, replayEnabled: Bool) -> PostHogConfig {
        let config = PostHogConfig(projectToken: apiKey, host: host)
        config.captureApplicationLifecycleEvents = true
        config.captureScreenViews = false
        config.captureElementInteractions = false
        config.sessionReplay = recordsSessions(environment: environment, replayEnabled: replayEnabled)
        config.sessionReplayConfig.screenshotMode = true
        config.sessionReplayConfig.screenshotModeBackgroundCapture = false
        config.sessionReplayConfig.maskAllTextInputs = true
        config.sessionReplayConfig.maskAllImages = true
        config.sessionReplayConfig.maskAllSandboxedViews = true
        config.sessionReplayConfig.captureNetworkTelemetry = false
        config.sessionReplayConfig.captureLogs = false
        config.personProfiles = .identifiedOnly
        return config
    }

    @MainActor
    init(apiKey: String, environment: MonacoEnvironment, replayEnabled: Bool) {
        let config = Self.makeConfig(apiKey: apiKey, environment: environment, replayEnabled: replayEnabled)
        SessionReplayGate.isRecording = config.sessionReplay
        PostHogSDK.shared.setup(config)
        PostHogSDK.shared.register(Self.superProperties(for: environment))
        if !AnalyticsConsent.isSharing { Self.apply(sharing: false) }
    }

    static func apply(sharing: Bool) {
        if sharing {
            PostHogSDK.shared.optIn()
        } else {
            PostHogSDK.shared.optOut()
        }
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
        if !AnalyticsConsent.isSharing { Self.apply(sharing: false) }
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

enum SessionReplayGate {
    static var isRecording = false
    private static var pauseDepth = 0

    static func pause() {
        pauseDepth += 1
        guard isRecording, pauseDepth == 1 else { return }
        PostHogSDK.shared.stopSessionRecording()
    }

    static func resume() {
        pauseDepth = max(0, pauseDepth - 1)
        guard isRecording, pauseDepth == 0 else { return }
        PostHogSDK.shared.startSessionRecording(resumeCurrent: true)
    }
}
