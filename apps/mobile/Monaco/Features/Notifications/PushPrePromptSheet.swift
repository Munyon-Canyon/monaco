import MonacoCore
import SwiftUI

struct PushPrePromptSheet: View {
    let prompt: PushPrePrompt

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            Text("Hear how votes end?")
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
                .multilineTextAlignment(.leading)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityAddTraits(.isHeader)
                .accessibilityIdentifier("push-pre-prompt-title")
            Text("You get the result when a vote closes. That\u{2019}s it.")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.leading)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
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
            ZStack(alignment: .bottomTrailing) {
                CabalMark(
                    groupId: "push-sample", name: "Cabal", size: .row,
                    pictureUrl: sample.cabalImageURL?.absoluteString)
                appBadge
            }
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
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .padding(MonacoTheme.Space.sm)
        .background(
            MonacoTheme.surface,
            in: RoundedRectangle(
                cornerRadius: MonacoTheme.Radius.compact, style: .continuous)
        )
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Example notification. \(sample.title). \(sample.body)")
        .accessibilityIdentifier("push-pre-prompt-sample")
    }

    private var appBadge: some View {
        RoundedRectangle(cornerRadius: MonacoTheme.Space.xs, style: .continuous)
            .fill(MonacoTheme.brandFill)
            .frame(width: 20, height: 20)
            .overlay {
                Image("LaunchMark")
                    .renderingMode(.template)
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .foregroundStyle(.white)
                    .padding(MonacoTheme.Space.xs)
            }
            .offset(x: MonacoTheme.Space.xs, y: MonacoTheme.Space.xs)
            .accessibilityHidden(true)
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
