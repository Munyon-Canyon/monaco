import MonacoAPI
import MonacoCore
import SwiftUI

struct FeedView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: FeedModel?
    private let makeModel: @MainActor (AppEnvironment) -> FeedModel

    init(makeModel: @escaping @MainActor (AppEnvironment) -> FeedModel = FeedView.liveModel) {
        self.makeModel = makeModel
    }

    var body: some View {
        ZStack {
            MonacoTheme.canvas.ignoresSafeArea()
            if let model {
                FeedScreen(model: model)
            } else {
                FeedSkeleton()
                    .frame(maxHeight: .infinity, alignment: .top)
            }
        }
        .monacoTopLevelHeader(title: FeedTab.title)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-root")
        .task { await start() }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private func start() async {
        let model = preparedModel()
        await model.load()
        await model.observe()
    }

    private func preparedModel() -> FeedModel {
        if let model { return model }
        let created = makeModel(environment)
        model = created
        return created
    }

    static func liveModel(_ environment: AppEnvironment) -> FeedModel {
        FeedModel(
            api: environment.api,
            viewerID: environment.viewer?.userID,
            hints: environment.hints,
            clock: ContinuousClock()
        )
    }
}

struct FeedSkeleton: View {
    var body: some View {
        VStack(spacing: 0) {
            ForEach(0..<FeedScreenMetrics.skeletonRows, id: \.self) { _ in FeedSkeletonRow() }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading the feed")
        .accessibilityIdentifier("feed-loading")
    }
}

private struct FeedSkeletonRow: View {
    var body: some View {
        VStack(spacing: 0) {
            HStack(alignment: .top, spacing: FeedPostMetrics.gap) {
                SkeletonBlock(
                    width: FeedPostMetrics.lead, height: FeedPostMetrics.lead,
                    radius: FeedPostMetrics.lead / 2)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    SkeletonFraction(0.5) { SkeletonBar() }
                    SkeletonFraction(0.8) { SkeletonBar() }
                    SkeletonBlock(height: FeedScreenMetrics.attachmentHeight, radius: MonacoTheme.Radius.field)
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.sm)
            MonacoRule(color: MonacoTheme.hairline)
                .padding(.leading, FeedPostMetrics.textColumn)
        }
    }
}

private struct FeedScreen: View {
    let model: FeedModel

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var replying: FeedReply?
    @State private var readingID: Components.Schemas.FeedItem.ID?
    @State private var showsMuted = false

    var body: some View {
        ScrollView {
            content
                .padding(.bottom, MonacoTheme.Space.m)
        }
        .scrollPosition(id: $readingID, anchor: .top)
        .refreshable { await model.refresh() }
        .monacoCanvas()
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) { filter }
        }
        .navigationDestination(isPresented: $showsMuted) { FeedMutesView() }
        .sheet(item: $replying) { FeedReplySheet(reply: $0) }
        .onChange(of: model.failureTick) { _, _ in
            guard let error = model.lastError else { return }
            toasts.show(error)
        }
        .onChange(of: model.reactions.noticeTick) { _, _ in
            guard let notice = model.reactions.notice else { return }
            toasts.current = MonacoToast(message: notice)
            model.reactions.dismissNotice()
        }
    }

    private func startReply(_ item: Components.Schemas.FeedItem) {
        let comments = CommentsModel.forPost(
            item: item, api: environment.api, hints: environment.hints, clock: ContinuousClock(),
            onPosted: { [weak model] in model?.didReply(to: item.id) })
        comments.presentReplySheet(.post(item))
        replying = FeedReply(item: item, comments: comments)
    }

    private var filter: some View {
        Menu {
            Picker("From", selection: scope) {
                ForEach(FeedScope.menu, id: \.self) { scope in
                    Text(scope.title).tag(scope).accessibilityIdentifier("feed-scope-\(scope.title)")
                }
            }
            .pickerStyle(.inline)
            .accessibilityIdentifier("feed-scope")
            Section {
                Button("Muted") { showsMuted = true }
                    .accessibilityIdentifier("feed-muted-link")
            }
        } label: {
            MonacoGlassButton(
                icon: .filter, accessibilityLabel: "Filter feed", style: isFiltered ? .selected : .glass,
                placement: .toolbar
            ) {}
        }
        .accessibilityLabel("Filter feed")
        .accessibilityIdentifier("feed-filter")
    }

    private var isFiltered: Bool {
        model.query.scope != .network
    }

    private func mute(_ option: FeedMuteOption) {
        Task {
            guard let receipt = await model.mute(option) else { return }
            toasts.show(
                success: receipt.message,
                action: MonacoToastAction(title: "Undo") { Task { await model.undo(receipt) } })
        }
    }

    private var scope: Binding<FeedScope> {
        Binding(
            get: { model.query.scope },
            set: { next in
                readingID = nil
                Task { await model.select(next) }
            }
        )
    }

    @ViewBuilder private var content: some View {
        if model.isQuietWeek {
            quietWeek
        } else {
            phased
        }
    }

    @ViewBuilder private var phased: some View {
        switch model.phase {
        case .loading:
            FeedSkeleton()
        case .empty:
            emptyState
        case .followsNobody:
            VStack(spacing: 0) {
                EmptyState(title: "Follow people to see what they do.", systemImage: MonacoIcon.profile.symbol)
                NavigationLink("Find friends", value: AnyAppRoute(FriendsRoute()))
                    .buttonStyle(.monacoSecondary)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .accessibilityIdentifier("feed-find-friends")
            }
            .frame(maxWidth: .infinity)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("feed-follow-nobody")
        case .failed:
            EmptyState(
                title: "Couldn’t load this view",
                message: "Your existing information hasn’t been changed. Try again when your connection is available.",
                actionTitle: "Try again", actionIdentifier: "feed-error", systemImage: MonacoIcon.offline.symbol,
                action: { Task { await model.reload() } }
            )
        case .loaded:
            timeline
        }
    }

    private var emptyState: some View {
        EmptyState(
            title: "Nothing here yet", message: "Change the filter to see more.",
            systemImage: MonacoIcon.filter.symbol
        )
        .accessibilityIdentifier("feed-empty")
    }

    private var quietWeek: some View {
        VStack(spacing: 0) {
            FeedQuietWeek { scope.wrappedValue = .everyone }
            LazyVStack(spacing: 0) {
                ForEach(model.fromEveryone) { item in
                    row(item, now: Date(), style: .outside)
                }
            }
            if !model.items.isEmpty {
                MonacoSectionHeader("Earlier")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .accessibilityIdentifier("feed-earlier")
                timeline
            }
        }
    }

    private func row(_ item: Components.Schemas.FeedItem, now: Date, style: FeedPostStyle) -> some View {
        VStack(spacing: 0) {
            FeedPost(item: item, now: now, style: style) {
                PostReactionsSlot(item: item, model: model, onReply: startReply)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRule(color: MonacoTheme.hairline)
                .padding(.leading, FeedPostMetrics.textColumn)
        }
    }

    private var timeline: some View {
        let items = model.items
        let now = Date()
        return VStack(spacing: 0) {
            LazyVStack(spacing: 0) {
                ForEach(Array(items.enumerated()), id: \.element.id) { index, item in
                    row(item, now: now, style: .standard)
                        .contextMenu {
                            ForEach(model.muteOptions(for: item)) { option in
                                Button(option.menuTitle) { mute(option) }
                            }
                        }
                        .onAppear {
                            guard index >= items.count - 5, !model.loadMoreFailed else { return }
                            Task { await model.loadMore() }
                        }
                }
            }
            .scrollTargetLayout()
            footer
                .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }

    @ViewBuilder private var footer: some View {
        if model.isLoadingMore {
            FeedSkeletonRow()
        } else if model.loadMoreFailed {
            VStack(spacing: MonacoTheme.Space.sm) {
                MonacoNotice(.warn, message: "Older updates couldn’t load. The items above are still here.")
                Button("Retry older updates") { Task { await model.loadMore() } }
                    .buttonStyle(.monacoOutline)
                    .accessibilityIdentifier("feed-load-more-retry")
            }
        } else if model.pager.phase == .exhausted {
            Text(isFiltered ? "No more matching updates" : "You’re caught up")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(maxWidth: .infinity, minHeight: FeedScreenMetrics.footerHeight)
                .accessibilityIdentifier("feed-caught-up")
        }
    }
}

private struct FeedReply: Identifiable {
    let item: Components.Schemas.FeedItem
    let comments: CommentsModel

    var id: String { item.id }
}

private struct FeedReplySheet: View {
    let reply: FeedReply

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ReplySheet(model: reply.comments, target: .post(reply.item))
            .onChange(of: reply.comments.replySheet) { _, target in
                if target == nil { dismiss() }
            }
    }
}

enum FeedScreenMetrics {
    static let footerHeight: CGFloat = 48
    static let attachmentHeight: CGFloat = 64
    static let skeletonRows = 4
}

#if DEBUG
final class FeedSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-feedHarness") else { return nil }
        let raw = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "items"
        var script = SampleAPIScript()
        script.proposal = .sample(canVote: true)
        SampleAPIProtocol.install(script)
        return AnyView(FeedHarnessScreen(mode: FeedModel.PreviewMode(rawValue: raw) ?? .items, auth: auth))
    }
}

private struct FeedHarnessScreen: View {
    let mode: FeedModel.PreviewMode

    @State private var environment: AppEnvironment

    init(mode: FeedModel.PreviewMode, auth: PrivyAuthService) {
        self.mode = mode
        _environment = State(initialValue: .sample(auth: auth, suite: "MonacoFeedHarness"))
    }

    var body: some View {
        NavigationStack(path: Bindable(environment.navigator).feedPath) {
            FeedView(makeModel: { _ in .preview(mode, clock: ContinuousClock()) })
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .defaultScrollAnchor(SampleScrollAnchor.requested(by: SampleScrollAnchor.flag))
        .environment(environment)
        .environment(environment.sessionStore)
    }
}
#endif
