import Foundation

nonisolated enum AnalyticsConsent {
    static let key = "analytics-sharing"
    static let title = "Share usage analytics"
    static let subtitle = "Helps us improve Monaco. Turn off to stop sending usage data."

    static func isSharing(in defaults: UserDefaults = .standard) -> Bool {
        defaults.object(forKey: key) as? Bool ?? true
    }

    static var isSharing: Bool { isSharing(in: .standard) }

    static func set(sharing: Bool, in defaults: UserDefaults = .standard) {
        defaults.set(sharing, forKey: key)
        PostHogAnalyticsSink.apply(sharing: sharing)
    }
}
