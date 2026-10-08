import AuthenticationServices
import SwiftUI

struct AppleSignInCapsule: View {
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        AppleSignInButtonRepresentable(style: colorScheme == .dark ? .white : .whiteOutline)
            .id(colorScheme)
            .accessibilityHidden(true)
    }
}

private struct AppleSignInButtonRepresentable: UIViewRepresentable {
    let style: ASAuthorizationAppleIDButton.Style

    func makeUIView(context: Context) -> ASAuthorizationAppleIDButton {
        let button = ASAuthorizationAppleIDButton(type: .signIn, style: style)
        button.cornerRadius = MonacoButtonMetrics.minimumHeight / 2
        return button
    }

    func updateUIView(_ uiView: ASAuthorizationAppleIDButton, context: Context) {}
}
