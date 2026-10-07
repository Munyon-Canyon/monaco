import MonacoCore
import PostHog

enum ReferralAnalytics {
    static func capture(_ event: ReferralAppEvent) {
        PostHogSDK.shared.capture(event.rawValue)
    }
}
