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
        .onChange(of: model?.membershipChanges) { _, _ in
            retry.retry?()
        }
    }

    @ViewBuilder
    private func content(_ model: CabalAccessModel) -> some View {
        switch model.standing {
        case .hidden:
            EmptyView()
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
        .monacoFullWidthButtons()
        .disabled(model.isBusy)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityIdentifier("cabal-join-button")
    }

    private func start() async {
        let model = self.model ?? CabalAccessModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        self.model = model
        refresh?.register("cabal-join") { await model.load() }
        await model.load()
        await model.observe()
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
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
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
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("cabal-join-request-row")
            if offersVote {
                Toggle(CabalJoinCopy.canVote, isOn: $canVote)
                    .font(MonacoTheme.Typo.rowTitle)
                    .tint(MonacoTheme.brandFill)
                    .frame(minHeight: 44)
                    .disabled(isDeciding)
                    .accessibilityIdentifier("cabal-join-can-vote")
            }
            ViewThatFits(in: .horizontal) {
                HStack(spacing: MonacoTheme.Space.sm) { buttons }
                VStack(spacing: MonacoTheme.Space.s) { buttons }
            }
            .disabled(isDeciding)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
    }

    @ViewBuilder
    private var buttons: some View {
        Button("Approve") { decide(true, offersVote && canVote) }
            .buttonStyle(.monacoPrimary)
            .accessibilityLabel("Approve \(request.name)")
            .accessibilityIdentifier("cabal-join-approve")
        Button("Deny") { decide(false, false) }
            .buttonStyle(.monacoSecondary)
            .accessibilityLabel("Deny \(request.name)")
            .accessibilityIdentifier("cabal-join-deny")
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
