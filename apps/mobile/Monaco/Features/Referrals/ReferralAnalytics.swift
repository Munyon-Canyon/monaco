import MonacoCore

enum ReferralAnalytics {
    @MainActor static func capture(_ event: ReferralAppEvent) {
        AppAnalytics.capture(event)
    }
}
