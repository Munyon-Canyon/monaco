import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalJoinSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalJoinSection(cabalID: context.cabalID)
    }
}

struct CabalJoinSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalRetry) private var retry
    @Environment(\.cabalModel) private var cabalModel
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    let cabalID: String
    @State private var model: CabalAccessModel?

    init(cabalID: String, model: CabalAccessModel? = nil) {
        self.cabalID = cabalID
        _model = State(initialValue: model)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if let model {
                content(model)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-join")
        .task(id: cabalID) { await start() }
        .task(id: cabalModel?.cabal) {
            guard let cabal = cabalModel?.cabal else { return }
            await preparedModel().load(cabal: cabal)
        }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: model?.toast) { _, toast in
            guard let toast else { return }
            toasts.current = MonacoToast(message: toast.message, isSuccess: toast.isSuccess)
            if toast.message == CabalEntry.requestedToast || toast.message == CabalEntry.joinedToast {
                Task { await environment.pushPrePrompt.noteCabalJoined(after: toasts) }
            }
        }
        .onChange(of: model?.membershipChanges) { previous, _ in
            guard previous != nil else { return }
            retry.retry?()
        }
    }

    @ViewBuilder
    private func content(_ model: CabalAccessModel) -> some View {
        switch model.standing {
        case .hidden:
            if model.loadFailed {
                MonacoErrorRow(thing: "your invite", identifier: "cabal-join-failed") {
                    Task { await model.load() }
                }
            }
        case .pending(let requests):
            NavigationLink(value: AnyAppRoute(MembersRoute(cabalID: cabalID, tab: .requests))) {
                MonacoRow(
                    title: "Join requests", subtitle: CabalCopy.requestsLine(requests.count), chevron: true,
                    isLast: true
                ) {
                    SunkenGlyphMark(systemImage: "person")
                }
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("cabal-join-requests-heading")
        case .invited, .join, .declined, .requested:
            CabalEntryActions(model: model)
                .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }

    private func preparedModel() -> CabalAccessModel {
        if let model { return model }
        let created = CabalAccessModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        model = created
        return created
    }

    private func start() async {
        let model = preparedModel()
        let shared = cabalModel
        refresh?.register("cabal-join") {
            if let cabal = shared?.cabal { await model.load(cabal: cabal) } else { await model.load() }
        }
        if shared == nil { await model.load() }
        await model.observe()
    }
}

struct CabalEntryActions: View {
    let model: CabalAccessModel

    var body: some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            switch model.standing {
            case .join:
                policyNote
                enterButton(idle: model.joinPolicy == .open ? "Join" : "Request to join")
            case .declined:
                declinedNote
                enterButton(idle: model.joinPolicy == .open ? "Join" : CabalJoinCopy.askAgain)
            case .requested:
                requestedButtons
            case .invited:
                inviteButtons
            case .hidden, .pending:
                EmptyView()
            }
        }
    }

    private var policyNote: some View {
        Text(model.joinPolicy.prospectNote)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityIdentifier("cabal-join-policy")
    }

    private var declinedNote: some View {
        Text(CabalJoinCopy.declined)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityIdentifier("cabal-join-declined")
    }

    private func enterButton(idle: String) -> some View {
        Button {
            Task { await model.enter() }
        } label: {
            SubmitLabel(
                isWorking: model.isBusy, idle: idle, working: model.joinPolicy == .open ? "Joining…" : "Sending…")
        }
        .buttonStyle(.monacoPrimary)
        .disabled(model.isBusy)
        .accessibilityIdentifier("cabal-join-button")
    }

    @ViewBuilder private var requestedButtons: some View {
        Button("Request pending") {}
            .buttonStyle(.monacoSecondary)
            .disabled(true)
            .accessibilityIdentifier("cabal-join-requested")
        Button("Cancel request") {
            Task { await model.cancelRequest() }
        }
        .buttonStyle(.monacoText)
        .disabled(model.isBusy)
        .accessibilityIdentifier("cabal-join-cancel")
    }

    @ViewBuilder private var inviteButtons: some View {
        Button {
            Task { await model.accept() }
        } label: {
            SubmitLabel(isWorking: model.isBusy, idle: "Accept invite", working: "Joining…")
        }
        .buttonStyle(.monacoPrimary)
        .disabled(model.isBusy)
        .accessibilityIdentifier("cabal-join-accept")
        Button("Decline") {
            Task { await model.decline() }
        }
        .buttonStyle(.monacoOutline)
        .disabled(model.isBusy)
        .accessibilityIdentifier("cabal-join-decline")
    }
}

enum CabalJoinCopy {
    static let canVote = "Can vote"
    static let declined = "Request declined"
    static let askAgain = "Ask again"
}

#if DEBUG
final class CabalJoinSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-MonacoCabalJoinSample"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        var cabal = Components.Schemas.Cabal.sample(role: nil)
        switch arguments[flag + 1] {
        case "request": break
        case "creator": cabal = .sample(role: "creator")
        default: return nil
        }
        return AnyView(
            NavigationStack {
                ScrollView {
                    CabalJoinSection(cabalID: cabal.id, model: .preview(cabal: cabal))
                        .padding(.top, MonacoTheme.Space.m)
                }
                .monacoCanvas()
                .navigationTitle(cabal.name)
                .navigationBarTitleDisplayMode(.inline)
            }
        )
    }
}
#endif
