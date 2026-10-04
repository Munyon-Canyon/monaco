import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalsInvitesSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        CabalInvitesSection()
    }
}

struct CabalInvitesSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: CabalInvitesModel?

    init(model: CabalInvitesModel? = nil) {
        _model = State(initialValue: model)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if let model, !model.invites.isEmpty {
                MonacoSectionHeader("Cabal invites", count: model.invites.count)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoGroupedList {
                    ForEach(model.invites) { invite in
                        CabalInviteRow(
                            invite: invite,
                            isAnswering: model.answering.contains(invite.id),
                            accept: { accept(invite, model) },
                            decline: { Task { await model.decline(invite) } }
                        )
                    }
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-invites")
        .task { await start() }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: model?.toast) { _, toast in
            guard let toast else { return }
            toasts.current = MonacoToast(message: toast.message, isSuccess: toast.isSuccess)
        }
    }

    private func start() async {
        let model = preparedModel()
        refresh?.register("cabal-invites") { await model.load() }
        if case .idle = model.state { await model.load() }
        await model.observe()
    }

    private func preparedModel() -> CabalInvitesModel {
        if let model { return model }
        let created = CabalInvitesModel(api: environment.api, hints: environment.hints)
        model = created
        return created
    }

    private func accept(_ invite: ReceivedCabalInvite, _ model: CabalInvitesModel) {
        Task {
            guard await model.accept(invite) else { return }
            Haptics.selection()
            environment.navigator.open(CabalRoute(id: invite.cabalID), in: .cabals)
        }
    }
}

private struct CabalInviteRow: View {
    let invite: ReceivedCabalInvite
    let isAnswering: Bool
    let accept: () -> Void
    let decline: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                CabalMark(groupId: invite.cabalID, name: invite.cabalName, pictureUrl: invite.pictureURL)
                VStack(alignment: .leading, spacing: 2) {
                    Text(invite.cabalName)
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.ink)
                    Text(invite.invitedBy)
                        .font(MonacoTheme.Typo.callout)
                        .foregroundStyle(MonacoTheme.ink)
                    Text(invite.members)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityElement(children: .combine)
            }
            ViewThatFits(in: .horizontal) {
                HStack(spacing: MonacoTheme.Space.sm) { buttons }
                VStack(spacing: MonacoTheme.Space.s) { buttons }
            }
            .disabled(isAnswering)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-invite-row")
    }

    @ViewBuilder
    private var buttons: some View {
        Button("Accept", action: accept)
            .buttonStyle(.monacoPrimary)
            .accessibilityLabel("Accept the invite to \(invite.cabalName)")
            .accessibilityIdentifier("cabal-invite-accept")
        Button("Decline", action: decline)
            .buttonStyle(.monacoSecondary)
            .accessibilityLabel("Decline the invite to \(invite.cabalName)")
            .accessibilityIdentifier("cabal-invite-decline")
    }
}

#if DEBUG
final class CabalInvitesSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-MonacoCabalInvitesSample"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains(launchArgument) else { return nil }
        return AnyView(
            NavigationStack {
                ScrollView {
                    CabalInvitesSection(model: CabalInvitesModel.preview())
                }
                .monacoCanvas()
                .navigationTitle(CabalsTab.title)
            }
        )
    }
}
#endif
