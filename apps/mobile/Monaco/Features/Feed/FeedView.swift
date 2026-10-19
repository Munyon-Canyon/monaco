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
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            SkeletonTitle()
            SkeletonFraction(0.4) { SkeletonBar() }
            SkeletonBlock(height: SkeletonMetrics.cardHeight, radius: MonacoTheme.Radius.cardTight)
            MonacoRowSkeleton(rows: 4, markShape: .circle)
                .monacoBleed()
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading the feed")
        .accessibilityIdentifier("feed-loading")
    }
}

private struct FeedScreen: View {
    let model: FeedModel

    @Environment(ToastCenter.self) private var toasts
    @Environment(\.feedOpensSoon) private var opensSoon
    @State private var search = ""
    @State private var readingID: Components.Schemas.FeedItem.ID?
    @State private var showsActivity = false
    @State private var showsMuted = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                MonacoSearchField(placeholder: "Search the feed", text: $search)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                content
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .scrollPosition(id: $readingID, anchor: .top)
        .scrollDismissesKeyboard(.interactively)
        .refreshable { await model.refresh() }
        .monacoCanvas()
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) { bell }
            if #available(iOS 26, *) {
                ToolbarSpacer(.fixed, placement: .topBarTrailing)
            }
            ToolbarItem(placement: .topBarTrailing) { filter }
        }
        .navigationDestination(isPresented: $showsActivity) { NotMigratedView(screen: "Activity") }
        .navigationDestination(isPresented: $showsMuted) { FeedMutesView() }
        .onChange(of: search) { _, text in
            readingID = nil
            model.setSearch(text)
        }
        .onChange(of: model.failureTick) { _, _ in
            guard let error = model.lastError else { return }
            toasts.show(error)
        }
    }

    private var bell: some View {
        MonacoGlassButton(
            icon: .bell, accessibilityLabel: "Activity notifications", placement: .toolbar
        ) {
            showsActivity = true
        }
        .overlay(alignment: .topTrailing) {
            if opensSoon.hasUnreadActivity {
                Circle()
                    .fill(MonacoTheme.brand)
                    .frame(width: FeedScreenMetrics.unreadDot, height: FeedScreenMetrics.unreadDot)
                    .padding(FeedScreenMetrics.unreadInset)
                    .accessibilityHidden(true)
            }
        }
        .accessibilityIdentifier("feed-bell")
    }

    private var filter: some View {
        Menu {
            Section("From") {
                Picker("From", selection: scope) {
                    ForEach(FeedScope.allCases, id: \.self) { scope in
                        Text(scope.title).tag(scope).accessibilityIdentifier("feed-scope-\(scope.title)")
                    }
                }
                .pickerStyle(.inline)
                .accessibilityIdentifier("feed-scope")
            }
            Section("Show") {
                Picker("Show", selection: chip) {
                    ForEach(FeedChip.allCases, id: \.self) { chip in
                        Text(chip.title).tag(chip).accessibilityIdentifier("feed-chip-\(chip.title)")
                    }
                }
                .pickerStyle(.inline)
            }
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
        .menuActionDismissBehavior(.disabled)
        .accessibilityLabel("Filter feed")
        .accessibilityIdentifier("feed-filter")
    }

    private var isFiltered: Bool {
        model.query.scope != .everyone || model.query.chip != .all
    }

    private var isNarrowed: Bool {
        isFiltered || !model.query.trimmedSearch.isEmpty
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

    private var chip: Binding<FeedChip> {
        Binding(
            get: { model.query.chip },
            set: { next in
                readingID = nil
                Task { await model.select(next) }
            }
        )
    }

    @ViewBuilder private var content: some View {
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
            cards
        }
    }

    @ViewBuilder private var emptyState: some View {
        if isNarrowed {
            EmptyState(
                title: "No matching updates", message: "Change the filter to see more activity.",
                systemImage: MonacoIcon.filter.symbol
            )
            .accessibilityIdentifier("feed-empty")
        } else {
            EmptyState(
                title: "No updates yet", message: "Proposals, trades and conversations will appear here.",
                systemImage: MonacoIcon.feed.symbol
            )
            .accessibilityIdentifier("feed-empty")
        }
    }

    private var cards: some View {
        let items = model.items
        return VStack(spacing: MonacoTheme.Space.sm) {
            LazyVStack(spacing: MonacoTheme.Space.sm) {
                ForEach(Array(items.enumerated()), id: \.element.id) { index, item in
                    FeedItemCell(item: item)
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
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    @ViewBuilder private var footer: some View {
        if model.isLoadingMore {
            SkeletonBlock(height: SkeletonMetrics.cardHeight, radius: MonacoTheme.Radius.cardTight)
        } else if model.loadMoreFailed {
            VStack(spacing: MonacoTheme.Space.sm) {
                MonacoNotice(.warn, message: "Older updates couldn’t load. The items above are still here.")
                Button("Retry older updates") { Task { await model.loadMore() } }
                    .buttonStyle(.monacoOutline)
                    .accessibilityIdentifier("feed-load-more-retry")
            }
        } else if model.pager.phase == .exhausted {
            Text(isNarrowed ? "No more matching updates" : "You’re caught up")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(maxWidth: .infinity, minHeight: FeedScreenMetrics.footerHeight)
                .accessibilityIdentifier("feed-caught-up")
        }
    }
}

enum FeedScreenMetrics {
    static let unreadDot: CGFloat = 6
    static let unreadInset: CGFloat = 7
    static let footerHeight: CGFloat = 48
}

#if DEBUG
final class FeedSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-feedHarness") else { return nil }
        let raw = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "items"
        return AnyView(FeedHarnessScreen(mode: FeedModel.PreviewMode(rawValue: raw) ?? .items))
    }
}

private struct FeedHarnessScreen: View {
    let mode: FeedModel.PreviewMode

    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            FeedView(makeModel: { _ in .preview(mode, clock: ContinuousClock()) })
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .environment(
            \.feedOpensSoon,
            FeedOpensSoonData(figures: FeedModel.previewFigures(mode), hasUnreadActivity: true)
        )
        .defaultScrollAnchor(SampleScrollAnchor.requested(by: SampleScrollAnchor.flag))
    }
}
#endif
