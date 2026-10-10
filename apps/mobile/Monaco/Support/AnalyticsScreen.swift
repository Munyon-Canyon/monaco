import MonacoAnalytics
import SwiftUI

extension EnvironmentValues {
    @Entry var analytics = Analytics(sink: NoopAnalyticsSink())
}

private struct AnalyticsScreenModifier: ViewModifier {
    let name: String
    let step: AnalyticsStep?
    @Environment(\.analytics) private var analytics
    @State private var firedStep = false

    func body(content: Content) -> some View {
        content.onAppear {
            analytics.screen(name)
            guard let step, !firedStep else { return }
            firedStep = true
            analytics.step(step)
        }
    }
}

extension Analytics {
    func loginStarted() {
        guard !hasAttempt(for: .onboarding) else { return }
        start(.onboarding)
        step(.onboarding(.loginStarted))
    }
}

extension View {
    func analyticsScreen(_ name: String, step: AnalyticsStep? = nil) -> some View {
        modifier(AnalyticsScreenModifier(name: name, step: step))
    }
}
