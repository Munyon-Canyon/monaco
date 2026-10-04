import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatComposerState: Equatable {
    case comingSoon
    case closed
}

struct CabalChatView: View {
    let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: CabalModel?

    static func composer(for cabal: Components.Schemas.Cabal) -> ChatComposerState {
        cabal.me == nil ? .closed : .comingSoon
    }

    var body: some View {
        CabalChatContent(state: model?.state ?? .loading) {
            Task { await model?.load() }
        }
        .task {
            let model = preparedModel()
            if model.cabal == nil { await model.load() }
            await model.observe()
        }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: model?.failureTick) { _, _ in
            guard model?.cabal != nil, let error = model?.lastError else { return }
            toasts.show(error)
        }
    }

    private func preparedModel() -> CabalModel {
        if let model { return model }
        let created = CabalModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}

private struct CabalChatContent: View {
    let state: LoadState<Components.Schemas.Cabal>
    let retry: () -> Void

    var body: some View {
        VStack(spacing: 0) {
            Spacer(minLength: 0)
            center
            Spacer(minLength: 0)
            if case .loaded(let cabal) = state {
                switch CabalChatView.composer(for: cabal) {
                case .comingSoon: ComingSoonComposer()
                case .closed: ClosedChatNotice()
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .monacoCanvas()
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if case .loaded(let cabal) = state {
                ToolbarItem(placement: .principal) {
                    HStack(spacing: MonacoTheme.Space.s) {
                        CabalMark(groupId: cabal.id, name: cabal.name, size: 24, pictureUrl: cabal.pictureUrl)
                        Text(cabal.name)
                            .font(MonacoTheme.Typo.rowTitle)
                            .foregroundStyle(MonacoTheme.ink)
                            .lineLimit(1)
                    }
                    .accessibilityElement(children: .combine)
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityIdentifier("chat-title")
                }
            }
        }
    }

    @ViewBuilder private var center: some View {
        switch state {
        case .idle, .loading:
            VStack(spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(width: 64, height: 64, radius: MonacoTheme.Radius.card)
                SkeletonBlock(width: 160, height: 24)
            }
            .accessibilityElement()
            .accessibilityLabel("Loading this cabal")
            .accessibilityIdentifier("chat-loading")
        case .failed:
            VStack(spacing: MonacoTheme.Space.sm) {
                Text("Couldn't load this cabal.")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.ink)
                Button("Try again", action: retry)
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("chat-retry")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("chat-failed")
        case .loaded(let cabal):
            VStack(spacing: MonacoTheme.Space.sm) {
                CabalMark(groupId: cabal.id, name: cabal.name, size: 64, pictureUrl: cabal.pictureUrl)
                Text(cabal.name)
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.ink)
                    .multilineTextAlignment(.center)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("chat-empty")
        }
    }
}

private struct ComingSoonComposer: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text("Chat opens soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("chat-composer-coming")
            HStack(alignment: .bottom, spacing: MonacoTheme.Space.s) {
                TextField(
                    "Message your cabal",
                    text: .constant(""),
                    prompt: Text("Message your cabal").foregroundStyle(MonacoTheme.disabledLabel)
                )
                .font(MonacoTheme.Typo.body)
                .padding(.horizontal, MonacoTheme.Space.m)
                .padding(.vertical, 11)
                .frame(minHeight: 44)
                .background(MonacoTheme.surfaceSunken, in: RoundedRectangle(cornerRadius: 22, style: .continuous))
                .disabled(true)
                .accessibilityIdentifier("chat-composer")
                Button {
                } label: {
                    ComposerSendDisc(isLive: false, isSending: false)
                }
                .buttonStyle(.plain)
                .disabled(true)
                .accessibilityLabel("Send message")
                .accessibilityIdentifier("chat-send")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.s)
        .background(MonacoTheme.background)
        .overlay(alignment: .top) { MonacoRule() }
    }
}

private struct ClosedChatNotice: View {
    var body: some View {
        Text("You're no longer in this cabal, so its chat is closed to you.")
            .font(MonacoTheme.Typo.callout)
            .foregroundStyle(MonacoTheme.muted)
            .multilineTextAlignment(.center)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.m)
            .background(MonacoTheme.background)
            .overlay(alignment: .top) { MonacoRule() }
            .accessibilityIdentifier("chat-closed")
    }
}

#if DEBUG
final class CabalChatSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-cabalChatHarness") else { return nil }
        let variant = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "member"
        let cabal = Components.Schemas.Cabal.sample(role: variant == "closed" ? nil : "member")
        return AnyView(
            NavigationStack {
                CabalChatContent(state: .loaded(cabal), retry: {})
            }
        )
    }
}
#endif
