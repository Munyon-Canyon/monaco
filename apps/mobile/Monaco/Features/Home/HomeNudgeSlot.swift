import SwiftUI

enum HomeNudgeSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        OnboardingNudgeBanner()
    }
}
