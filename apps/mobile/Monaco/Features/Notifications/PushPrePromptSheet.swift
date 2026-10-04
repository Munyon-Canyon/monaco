import MonacoCore
import SwiftUI

struct PushPrePromptSheet: View {
    let prompt: PushPrePrompt

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        ScrollView {
            VStack(spacing: MonacoTheme.Space.m) {
                SunkenGlyphMark(systemImage: "bell", size: 64)
                Text("Know when your cabal votes and trades")
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.ink)
                    .multilineTextAlignment(.center)
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityIdentifier("push-pre-prompt-title")
                Text("We'll tell you when a vote opens, passes, or a trade fills.")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.muted)
                    .multilineTextAlignment(.center)
                VStack(spacing: MonacoTheme.Space.s) {
                    Button {
                        Task { await prompt.turnOn() }
                    } label: {
                        Text("Turn on notifications").lineLimit(2).multilineTextAlignment(.center)
                    }
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("push-pre-prompt-turn-on")
                    Button("Not now", action: prompt.notNow)
                        .buttonStyle(.monacoSecondary)
                        .accessibilityIdentifier("push-pre-prompt-not-now")
                }
                .monacoFullWidthButtons()
                .padding(.top, MonacoTheme.Space.s)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xl)
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoCanvas()
        .presentationDetents(dynamicTypeSize.isAccessibilitySize ? [.large] : [.medium])
        .presentationDragIndicator(.visible)
        .accessibilityIdentifier("push-pre-prompt")
    }
}
