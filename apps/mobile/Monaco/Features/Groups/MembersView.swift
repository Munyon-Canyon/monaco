import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated enum MembersTab: String, CaseIterable, Hashable, Sendable {
    case members
    case voters
    case requests

    var title: String {
        switch self {
        case .members: "Members"
        case .voters: "Voters"
        case .requests: "Requests"
        }
    }
}

nonisolated struct MembersRoute: AppRoute {
    let cabalID: String
    var tab: MembersTab = .members

    @MainActor func destination() -> some View {
        MembersView(cabalID: cabalID, initialTab: tab)
    }
}

struct MembersView: View {
    let cabalID: String
    var initialTab: MembersTab = .members
    var access: CabalAccessModel?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var models = CabalScreenModels()
    @State private var tab: MembersTab
    @State private var requests: CabalAccessModel?
    @State private var showsVoters = false
    @State private var reviewing: CabalPendingRequest?
    @State private var retryTick = 0

    init(cabalID: String, initialTab: MembersTab = .members, access: CabalAccessModel? = nil) {
        self.cabalID = cabalID
        self.initialTab = initialTab
        self.access = access
        _tab = State(initialValue: initialTab)
        _requests = State(initialValue: access)
    }

    var body: some View {
        let (cabal, _) = models.prepared(cabalID: cabalID, environment: environment)
        content(cabal)
            .navigationTitle("Members & voters")
            .navigationBarTitleDisplayMode(.inline)
            .monacoCanvas()
            .task(id: retryTick) {
                await cabal.load()
                await cabal.observe()
            }
            .task(id: cabal.cabal) {
                guard let loaded = cabal.cabal, loaded.me?.role == "creator" else { return }
                let model =
                    requests
                    ?? CabalAccessModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
                requests = model
                await model.load(cabal: loaded)
            }
            .onScreenVisibilityChange { cabal.setVisible($0) }
            .onChange(of: requests?.toast) { _, toast in
                guard let toast else { return }
                toasts.current = MonacoToast(message: toast.message, isSuccess: toast.isSuccess)
            }
            .onChange(of: requests?.membershipChanges) { previous, _ in
                guard previous != nil else { return }
                Task { await cabal.load() }
            }
    }

    @ViewBuilder private func content(_ cabal: CabalModel) -> some View {
        switch cabal.state {
        case .idle, .loading:
            MembersSkeleton()
        case .failed:
            CabalFullScreenError(identifier: "members-failed") { retryTick += 1 }
        case .loaded(let loaded):
            loadedContent(loaded, cabal: cabal)
        }
    }

    private func tabs(_ cabal: Components.Schemas.Cabal) -> [MembersTab] {
        cabal.me?.role == "creator" ? MembersTab.allCases : [.members, .voters]
    }

    private func loadedContent(_ cabal: Components.Schemas.Cabal, cabal model: CabalModel) -> some View {
        let shownTabs = tabs(cabal)
        return ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                MonacoSegmented(shownTabs, selection: $tab, identifier: { "members-tab-\($0.rawValue)" }) {
                    $0.title
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.sm)
                switch shownTabs.contains(tab) ? tab : .members {
                case .members: membersList(cabal)
                case .voters: votersList(cabal, model: model)
                case .requests: requestsList(cabal)
                }
            }
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .refreshable { await model.load() }
        .sheet(isPresented: $showsVoters) {
            ChooseVotersSheet(cabal: cabal) { await model.load() }
        }
        .confirmationDialog(
            reviewing?.name ?? "", isPresented: reviewingBinding, titleVisibility: .visible, presenting: reviewing
        ) { request in
            reviewButtons(request)
        }
    }

    private var reviewingBinding: Binding<Bool> {
        Binding(get: { reviewing != nil }, set: { if !$0 { reviewing = nil } })
    }

    private func helper(_ cabal: Components.Schemas.Cabal) -> some View {
        Text(
            CabalCopy.votersSummary(members: cabal.memberCount, eligible: CabalCopy.eligibleVoterCount(cabal))
        )
        .font(MonacoTheme.Typo.caption)
        .foregroundStyle(MonacoTheme.secondaryText)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
        .accessibilityIdentifier("members-helper")
    }

    private func role(_ member: Components.Schemas.CabalMember) -> String {
        member.role == "creator" ? "Admin" : "Member"
    }

    private func membersList(_ cabal: Components.Schemas.Cabal) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            helper(cabal)
            MonacoGroupedList {
                ForEach(cabal.members, id: \.userId) { member in
                    NavigationLink(
                        value: AnyAppRoute(
                            UserProfileRoute(
                                userID: member.userId,
                                preview: UserPreview(
                                    displayName: member.shownName, handle: member.handle, photoURL: member.photoUrl)
                            ))
                    ) {
                        MonacoRow(
                            title: member.shownName, subtitle: role(member), chevron: true,
                            isLast: member.userId == cabal.members.last?.userId, isPrivate: true
                        ) {
                            MonacoAvatar(
                                photoURL: member.photoUrl, displayName: member.shownName, size: .row,
                                seed: member.userId)
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-member-\(member.userId)")
                }
            }
        }
        .accessibilityIdentifier("members-list")
    }

    private func votersList(_ cabal: Components.Schemas.Cabal, model: CabalModel) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            helper(cabal)
            MonacoGroupedList {
                ForEach(cabal.members, id: \.userId) { member in
                    MonacoRow(
                        title: member.shownName, subtitle: role(member),
                        isLast: member.userId == cabal.members.last?.userId, isPrivate: true
                    ) {
                        MonacoAvatar(
                            photoURL: member.photoUrl, displayName: member.shownName, size: .row, seed: member.userId)
                    } trailing: {
                        StatusBadge(
                            member.canVote ? "Voter" : "Not voting", tone: member.canVote ? .positive : .neutral)
                    }
                    .accessibilityIdentifier("voters-member-\(member.userId)")
                }
            }
            if cabal.me?.role == "creator", cabal.rules.voterMode == CabalVoterMode.picked.rawValue {
                Button("Choose voters") { showsVoters = true }
                    .buttonStyle(.monacoPrimary)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.m)
                    .accessibilityIdentifier("members-choose-voters")
            }
        }
    }

    @ViewBuilder private func requestsList(_ cabal: Components.Schemas.Cabal) -> some View {
        let pending: [CabalPendingRequest] = {
            if case .pending(let list)? = requests?.standing { return list }
            return []
        }()
        VStack(alignment: .leading, spacing: 0) {
            if requests?.picksVoters == true {
                MonacoNotice(
                    .info, message: "Accepting a member does not automatically grant voting eligibility."
                )
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.m)
                .accessibilityIdentifier("members-requests-notice")
            }
            if pending.isEmpty {
                if requests?.loadFailed == true {
                    MonacoErrorRow(thing: "join requests", identifier: "members-requests-failed") {
                        Task { await requests?.load() }
                    }
                } else if requests?.standing != nil {
                    EmptyState(
                        title: "No pending requests", message: "New membership requests will appear here.",
                        systemImage: "person.2"
                    )
                    .accessibilityIdentifier("members-requests-empty")
                }
            } else {
                MonacoGroupedList {
                    ForEach(pending) { request in
                        RequestRow(
                            request: request, isDeciding: requests?.deciding.contains(request.id) == true,
                            isLast: request.id == pending.last?.id
                        ) { reviewing = request }
                    }
                }
                .padding(.top, MonacoTheme.Space.s)
            }
        }
    }

    @ViewBuilder private func reviewButtons(_ request: CabalPendingRequest) -> some View {
        Button("Approve") {
            Task { await requests?.decide(request, approve: true) }
        }
        .accessibilityIdentifier("cabal-join-approve")
        if requests?.picksVoters == true {
            Button("Approve and let them vote") {
                Task { await requests?.decide(request, approve: true, canVote: true) }
            }
            .accessibilityIdentifier("cabal-join-approve-vote")
        }
        Button("Deny", role: .destructive) {
            Task { await requests?.decide(request, approve: false) }
        }
        .accessibilityIdentifier("cabal-join-deny")
        Button("Cancel", role: .cancel) {}
    }
}

private struct RequestRow: View {
    let request: CabalPendingRequest
    let isDeciding: Bool
    let isLast: Bool
    let review: () -> Void

    var body: some View {
        MonacoRow(
            title: request.name, subtitle: "Requested to join", isLast: isLast, trailingIsInteractive: true,
            isPrivate: true,
            leading: { token },
            trailing: {
                Button("Review", action: review)
                    .buttonStyle(.monacoCompact)
                    .disabled(isDeciding)
                    .accessibilityLabel("Review \(request.name)")
                    .accessibilityIdentifier("cabal-join-review")
            }
        )
        .accessibilityLabel("\(request.name), requested to join")
        .accessibilityIdentifier("cabal-join-request-row")
    }

    @ViewBuilder private var token: some View {
        if let photo = request.photoURL {
            MonacoAvatar(photoURL: photo, displayName: request.name, size: .row, seed: request.userID)
        } else {
            Text(CabalMark.initials(for: request.name))
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.brand)
                .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
                .background(Circle().fill(MonacoTheme.brandSoft))
                .accessibilityHidden(true)
        }
    }
}

private struct MembersSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            SkeletonBlock(height: MonacoSegmentedMetrics.minHeight, radius: MonacoSegmentedMetrics.minHeight / 2)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            SkeletonBar().padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRowSkeleton(rows: 4, markShape: .circle, hasTrailing: false)
            Spacer(minLength: 0)
        }
        .padding(.top, MonacoTheme.Space.sm)
        .accessibilityElement()
        .accessibilityLabel("Loading members")
        .accessibilityIdentifier("members-loading")
    }
}

struct ChooseVotersSheet: View {
    let cabal: Components.Schemas.Cabal
    let saved: () async -> Void

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @State private var edit: CabalEditModel?
    @State private var draft: CabalVoterChoice

    init(cabal: Components.Schemas.Cabal, saved: @escaping () async -> Void) {
        self.cabal = cabal
        self.saved = saved
        _draft = State(initialValue: CabalVoterChoice(cabal))
    }

    private var isSaving: Bool { edit?.isSaving == true }

    private var isChanged: Bool { draft != CabalVoterChoice(cabal) }

    var body: some View {
        ScrollView {
            CabalVoterChecklist(cabal: cabal, voters: $draft, isDisabled: isSaving)
                .padding(.bottom, MonacoTheme.Space.m)
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoSheet(title: "Eligible voters", detents: .votes, showsClose: true)
        .safeAreaInset(edge: .bottom) {
            Button {
                Task { await save() }
            } label: {
                SubmitLabel(isWorking: isSaving, idle: "Save voters", working: "Saving…")
            }
            .buttonStyle(.monacoPrimary)
            .disabled(!isChanged || isSaving)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.sm)
            .accessibilityIdentifier("edit-cabal-voters-save")
        }
        .task {
            let model = CabalEditModel(cabalID: cabal.id, api: environment.api, hints: environment.hints)
            edit = model
            await model.load()
        }
    }

    private func save() async {
        guard let edit, var settings = edit.settings else { return }
        settings.voters = draft
        switch await edit.save(settings) {
        case .saved:
            toasts.show(success: CabalCopy.votersSaved)
            await saved()
            dismiss()
        case .unchanged:
            dismiss()
        case .failed(let message):
            toasts.current = MonacoToast(message: message)
        }
    }
}
