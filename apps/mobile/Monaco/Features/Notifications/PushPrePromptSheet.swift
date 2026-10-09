import MonacoCore
import SwiftUI

struct PushPrePromptSheet: View {
    let prompt: PushPrePrompt

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        ScrollView {
            VStack(spacing: MonacoTheme.Space.m) {
                SunkenGlyphMark(systemImage: "bell", size: 64)
                Text("We'll tell you when a vote opens or passes, when a trade fills, and when a cabal lets you in.")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.muted)
                    .multilineTextAlignment(.center)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .scrollBounceBehavior(.basedOnSize)
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button("Not now", action: prompt.notNow)
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("push-pre-prompt-not-now")
                Button {
                    Task { await prompt.turnOn() }
                } label: {
                    Text("Turn on notifications").lineLimit(2).multilineTextAlignment(.center)
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("push-pre-prompt-turn-on")
            }
        }
        .monacoSheet(title: "Know when your cabal votes and trades", titleIdentifier: "push-pre-prompt-title")
        .presentationDetents(dynamicTypeSize.isAccessibilitySize ? [.large] : [.medium])
        .accessibilityIdentifier("push-pre-prompt")
    }
}
