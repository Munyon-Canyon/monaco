import MonacoCore
import SwiftUI

struct HandleStepView: View {
    let onContinue: () -> Void

    var body: some View {
        OnboardingStepScaffold(
            title: OnboardingCopy.handleTitle, identifier: "onboarding-handle-step", onContinue: onContinue)
    }
}
