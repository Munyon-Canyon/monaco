import SwiftUI

struct VerifyCodeHeader: View {
    let backIdentifier: String
    let onBack: () -> Void

    var body: some View {
        HStack(spacing: 0) {
            MonacoGlassButton(systemImage: "chevron.left", accessibilityLabel: "Back", action: onBack)
                .accessibilityIdentifier(backIdentifier)
            Text("Verify code")
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(.center)
                .frame(maxWidth: .infinity)
                .accessibilityAddTraits(.isHeader)
            Color.clear
                .frame(width: MonacoButtonMetrics.hitTarget, height: MonacoButtonMetrics.hitTarget)
                .accessibilityHidden(true)
        }
        .padding(.vertical, MonacoTheme.Space.sm)
    }
}

#Preview {
    VerifyCodeHeader(backIdentifier: "smsVerifyBackButton") {}
        .padding(.horizontal, MonacoTheme.Space.gutter)
}
