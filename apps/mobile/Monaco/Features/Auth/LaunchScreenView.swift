import SwiftUI

/// The top of sign-in: the wordmark, the headline and what you do in Monaco in two short lines.
struct LaunchScreenView: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoWordmark(.signIn, isTitle: true)
                .padding(.top, MonacoTheme.Space.m)

            Text("Your people.\nYour next chapter.")
                .authHeadline()
                .padding(.top, MonacoTheme.Space.xl + MonacoTheme.Space.l)

            Text("Pool funds. Propose an investment.\nDecide together.")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: 300, alignment: .leading)
                .padding(.top, MonacoTheme.Space.sm)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

#Preview {
    LaunchScreenView()
        .padding()
}
