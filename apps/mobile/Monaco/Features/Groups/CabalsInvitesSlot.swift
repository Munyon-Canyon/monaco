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
            if let model, case .failed = model.state {
                MonacoErrorRow(thing: "your invites", identifier: "cabals-invites-failed") {
                    Task { await model.load() }
                }
            } else if let model, !model.invites.isEmpty {
                MonacoSectionHeader("Cabal invites", count: model.invites.count)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoGroupedList {
                    ForEach(model.invites) { invite in
                        CabalInviteRow(
                            invite: invite,
                            isAnswering: model.answering.contains(invite.id),
                            isLast: invite.id == model.invites.last?.id,
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
            if toast.message == CabalEntry.joinedToast {
                Task { await environment.pushPrePrompt.noteCabalJoined(after: toasts) }
            }
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
    let isLast: Bool
    let accept: () -> Void
    let decline: () -> Void

    var body: some View {
        MonacoRow(
            title: invite.cabalName, subtitle: invite.invitedBy, isLast: isLast,
            trailingIsInteractive: true
        ) {
            CabalMark(groupId: invite.cabalID, name: invite.cabalName, pictureUrl: invite.pictureURL)
        } trailing: {
            HStack(spacing: MonacoTheme.Space.s) {
                Button(action: decline) {
                    Text("Decline")
                        .font(MonacoTheme.Typo.calloutStrong)
                        .foregroundStyle(MonacoTheme.muted)
                        .frame(minHeight: 44)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Decline the invite to \(invite.cabalName)")
                .accessibilityIdentifier("cabal-invite-decline")
                Button("Accept", action: accept)
                    .buttonStyle(.monacoCompactProminent)
                    .accessibilityLabel("Accept the invite to \(invite.cabalName)")
                    .accessibilityIdentifier("cabal-invite-accept")
            }
            .disabled(isAnswering)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-invite-row")
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
