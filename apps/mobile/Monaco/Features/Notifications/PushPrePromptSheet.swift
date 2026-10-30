import MonacoCore
import SwiftUI

struct PushPrePromptSheet: View {
    let prompt: PushPrePrompt

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            Image(systemName: "bell.badge")
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.accent)
                .frame(width: 56, height: 56)
                .background(MonacoTheme.brandSoft, in: Circle())
                .accessibilityHidden(true)
            Text("Hear how votes end?")
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .center)
                .accessibilityAddTraits(.isHeader)
                .accessibilityIdentifier("push-pre-prompt-title")
            Text("Get each result when a vote closes. No more vote reminders after you vote.")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .center)
            if let sample = prompt.sample { sampleCard(sample) }
            Button {
                Task { await prompt.turnOn() }
            } label: {
                Text("Turn on notifications").lineLimit(2).multilineTextAlignment(.center)
            }
            .buttonStyle(.monacoPrimary)
            .accessibilityIdentifier("push-pre-prompt-turn-on")
            textButtons {
                Button("Deliver quietly") { Task { await prompt.deliverQuietly() } }
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier("push-pre-prompt-quietly")
                Button("Not now", action: prompt.notNow)
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier("push-pre-prompt-not-now")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.l)
        .padding(.bottom, MonacoTheme.Space.m)
        .monacoFittedSheet(title: "Hear how votes end?", showsHeading: false)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("push-pre-prompt")
    }

    private func sampleCard(_ sample: PushSample) -> some View {
        let layout =
            dynamicTypeSize >= .xxxLarge
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.sm))
            : AnyLayout(HStackLayout(alignment: .top, spacing: MonacoTheme.Space.sm))
        return layout {
            CabalMark(
                groupId: "push-sample", name: "Cabal", size: .row, pictureUrl: sample.cabalImageURL?.absoluteString)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(sample.title)
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)
                Text(sample.body)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.bodyText)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Text("now")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        }
        .padding(MonacoTheme.Space.sm)
        .background(
            MonacoTheme.surface,
            in: RoundedRectangle(
                cornerRadius: MonacoTheme.Radius.cardTight, style: .continuous)
        )
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("push-pre-prompt-sample")
    }

    private func textButtons<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.m))
        return layout { content() }
            .frame(maxWidth: .infinity, alignment: .center)
    }
}
