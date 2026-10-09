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
        case .invited:
            CabalInviteAnswer(model: model)
        case .join:
            policyNote(model.joinPolicy)
            enterButton(model, idle: model.joinPolicy == .open ? "Join" : "Request to join")
        case .declined:
            MonacoGroupedList {
                MonacoRow(
                    title: CabalJoinCopy.declined,
                    subtitle: model.joinPolicy.prospectNote,
                    isLast: true,
                    leading: {
                        Image(systemName: "xmark.circle")
                            .font(.title3)
                            .foregroundStyle(MonacoTheme.muted)
                            .accessibilityHidden(true)
                    }
                )
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("cabal-join-declined")
            }
            enterButton(model, idle: model.joinPolicy == .open ? "Join" : CabalJoinCopy.askAgain)
        case .requested:
            MonacoGroupedList {
                MonacoRow(
                    title: "Request sent",
                    isLast: true,
                    leading: {
                        Image(systemName: "clock")
                            .font(.title3)
                            .foregroundStyle(MonacoTheme.warning)
                            .accessibilityHidden(true)
                    }
                )
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("cabal-join-requested")
            }
            Button("Cancel request") {
                Task { await model.cancelRequest() }
            }
            .buttonStyle(.monacoText)
            .disabled(model.isBusy)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("cabal-join-cancel")
        case .pending(let requests):
            MonacoSectionHeader(CabalAccessStanding.heading(count: requests.count))
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cabal-join-requests-heading")
            MonacoGroupedList {
                ForEach(requests) { request in
                    CabalPendingRequestRow(
                        request: request,
                        isDeciding: model.deciding.contains(request.id),
                        offersVote: model.picksVoters,
                        decide: { approve, canVote in
                            Task { await model.decide(request, approve: approve, canVote: canVote) }
                        }
                    )
                }
            }
        }
    }

    private func policyNote(_ policy: CabalJoinPolicy) -> some View {
        Text(policy.prospectNote)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
            .fixedSize(horizontal: false, vertical: true)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("cabal-join-policy")
    }

    private func enterButton(_ model: CabalAccessModel, idle: String) -> some View {
        Button {
            Task { await model.enter() }
        } label: {
            SubmitLabel(
                isWorking: model.isBusy, idle: idle, working: model.joinPolicy == .open ? "Joining…" : "Sending…")
        }
        .buttonStyle(.monacoPrimary)
        .disabled(model.isBusy)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityIdentifier("cabal-join-button")
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

private struct CabalInviteAnswer: View {
    let model: CabalAccessModel

    var body: some View {
        Button {
            Task { await model.accept() }
        } label: {
            SubmitLabel(isWorking: model.isBusy, idle: "Accept invite", working: "Joining…")
        }
        .buttonStyle(.monacoPrimary)
        .disabled(model.isBusy)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityIdentifier("cabal-join-accept")
        Button("Decline") {
            Task { await model.decline() }
        }
        .buttonStyle(.plain)
        .font(MonacoTheme.Typo.rowTitle)
        .foregroundStyle(MonacoTheme.muted)
        .frame(maxWidth: .infinity, minHeight: 44)
        .contentShape(Rectangle())
        .disabled(model.isBusy)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityIdentifier("cabal-join-decline")
    }
}

enum CabalJoinCopy {
    static let canVote = "Can vote"
    static let declined = "Request declined"
    static let askAgain = "Ask again"
}

private struct CabalPendingRequestRow: View {
    let request: CabalPendingRequest
    let isDeciding: Bool
    let offersVote: Bool
    let decide: (Bool, Bool) -> Void

    @State private var canVote = false

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: MonacoTheme.Space.s) {
                NavigationLink(
                    value: AnyAppRoute(
                        UserProfileRoute(
                            userID: request.userID,
                            preview: UserPreview(displayName: request.name, handle: nil, photoURL: request.photoURL)
                        ))
                ) {
                    HStack(spacing: MonacoTheme.Space.sm) {
                        MonacoAvatar(photoURL: request.photoURL, displayName: request.name, seed: request.userID)
                        Text(request.name)
                            .font(MonacoTheme.Typo.rowTitle)
                            .foregroundStyle(MonacoTheme.ink)
                            .lineLimit(1)
                            .truncationMode(.tail)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("cabal-join-request-row")
                HStack(spacing: MonacoTheme.Space.s) {
                    Button {
                        decide(false, false)
                    } label: {
                        Text("Deny")
                            .font(MonacoTheme.Typo.calloutStrong)
                            .foregroundStyle(MonacoTheme.muted)
                            .frame(minHeight: 44)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("Deny \(request.name)")
                    .accessibilityIdentifier("cabal-join-deny")
                    Button("Approve") { decide(true, offersVote && canVote) }
                        .buttonStyle(.monacoCompactProminent)
                        .accessibilityLabel("Approve \(request.name)")
                        .accessibilityIdentifier("cabal-join-approve")
                }
                .disabled(isDeciding)
            }
            if offersVote {
                Toggle(CabalJoinCopy.canVote, isOn: $canVote)
                    .font(MonacoTheme.Typo.rowTitle)
                    .tint(MonacoTheme.brandFill)
                    .frame(minHeight: 44)
                    .disabled(isDeciding)
                    .accessibilityIdentifier("cabal-join-can-vote")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
    }
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
