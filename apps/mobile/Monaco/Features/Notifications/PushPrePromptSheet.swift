import MonacoCore
import SwiftUI

struct PushPrePromptSheet: View {
    let prompt: PushPrePrompt

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var contentHeight: CGFloat = 0

    @ScaledMetric(relativeTo: .title) private var titleAllowance: CGFloat = 96

    var body: some View {
        ScrollView {
            VStack(spacing: MonacoTheme.Space.m) {
                Text("We'll tell you when a vote opens or passes, when a trade fills, and when a cabal lets you in.")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.muted)
                    .frame(maxWidth: .infinity, alignment: .leading)
                buttons {
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
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.m)
            .onGeometryChange(for: CGFloat.self) {
                $0.size.height
            } action: {
                contentHeight = $0
            }
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoSheet(title: "Know when your cabal votes and trades", titleIdentifier: "push-pre-prompt-title")
        .presentationDetents(contentHeight > 0 ? [.height(contentHeight + titleAllowance)] : [.medium])
        .accessibilityIdentifier("push-pre-prompt")
    }

    private func buttons<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        return layout { content() }
    }
}
