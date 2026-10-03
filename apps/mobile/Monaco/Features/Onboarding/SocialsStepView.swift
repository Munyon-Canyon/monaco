import MonacoCore
import SwiftUI

struct SocialsStepView: View {
    let onContinue: () -> Void

    var body: some View {
        OnboardingStepScaffold(
            title: OnboardingCopy.socialsTitle, identifier: "onboarding-socials-step", onContinue: onContinue)
    }
}
