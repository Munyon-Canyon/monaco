import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

enum CabalInviteCodeSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalInviteCodeSection(cabalID: context.cabalID)
    }
}

struct CabalInviteCodeSection: View {
    @Environment(AppEnvironment.self) private var environment
    let cabalID: String
    @State private var model: CabalModel?

    init(cabalID: String, model: CabalModel? = nil) {
        self.cabalID = cabalID
        _model = State(initialValue: model)
    }

    var body: some View {
        VStack(spacing: 0) {
            if let code = model?.cabal?.inviteCode {
                CabalInviteCodeCard(code: code)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
        .task(id: cabalID) {
            let model = self.model ?? CabalModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
            self.model = model
            await model.load()
            await model.observe()
        }
    }
}

struct CabalInviteCodeCard: View {
    static let copiedFor: Duration = .seconds(2)

    let code: String
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var copied = false

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(JoinCabalCopy.codeLabel)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.muted)
                Text(code)
                    .font(MonacoTheme.Typo.data)
                    .foregroundStyle(MonacoTheme.ink)
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
                    .textSelection(.enabled)
                    .accessibilityIdentifier("cabal-invite-code")
            }
            Text(CabalDetailsCopy.inviteHint)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
            let actions =
                dynamicTypeSize.isAccessibilitySize
                ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.sm))
                : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
            actions {
                Button(action: copy) {
                    Label(
                        copied ? CabalDetailsCopy.copied : CabalDetailsCopy.copyCode,
                        systemImage: copied ? "checkmark" : "doc.on.doc"
                    )
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("cabal-invite-copy")
                ShareLink(item: CabalCopy.inviteShareText(code: code)) {
                    Label(CabalDetailsCopy.share, systemImage: "square.and.arrow.up")
                }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("cabal-invite-share")
            }
            .monacoFullWidthButtons()
            .padding(.top, MonacoTheme.Space.xs)
        }
        .padding(MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            MonacoTheme.surface,
            in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
        )
        .overlay {
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                .strokeBorder(MonacoTheme.hairline, lineWidth: 1)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-invite-card")
    }

    private func copy() {
        UIPasteboard.general.string = code
        Haptics.selection()
        copied = true
        Task {
            try? await Task.sleep(for: Self.copiedFor)
            copied = false
        }
    }
}

#if DEBUG
final class CabalInviteCodeSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-MonacoCabalInviteCodeSample"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains(launchArgument) else { return nil }
        return AnyView(
            NavigationStack {
                ScrollView {
                    CabalInviteCodeCard(code: "ABCD2345XY")
                        .padding(MonacoTheme.Space.gutter)
                }
                .monacoCanvas()
                .navigationTitle("Cabal details")
                .navigationBarTitleDisplayMode(.inline)
            }
        )
    }
}
#endif
