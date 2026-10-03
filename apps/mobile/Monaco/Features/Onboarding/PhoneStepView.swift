import MonacoCore
import SwiftUI

struct PhoneStepView: View {
    let onContinue: () -> Void

    var body: some View {
        OnboardingStepScaffold(
            title: OnboardingCopy.phoneTitle, identifier: "onboarding-phone-step", onContinue: onContinue)
    }
}
