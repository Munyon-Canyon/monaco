import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct FeedItemRoute: AppRoute {
    let itemID: String
    var anchorCommentID: String?

    @MainActor func destination() -> PostScreen {
        PostScreen(itemID: itemID, anchorCommentID: anchorCommentID)
    }
}

struct PostScreen: View {
    let itemID: String
    var anchorCommentID: String?
    var startsAtEnd = false

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: FeedItemDetailModel?
    @State private var feed: FeedModel?
    private let makeModel: @MainActor (AppEnvironment, String, String?) -> FeedItemDetailModel
    private let makeFeed: @MainActor (AppEnvironment) -> FeedModel

    init(
        itemID: String,
        anchorCommentID: String? = nil,
        startsAtEnd: Bool = false,
        makeModel: @escaping @MainActor (AppEnvironment, String, String?) -> FeedItemDetailModel =
            PostScreen.liveModel,
        makeFeed: @escaping @MainActor (AppEnvironment) -> FeedModel = PostScreen.liveFeed
    ) {
        self.itemID = itemID
        self.anchorCommentID = anchorCommentID
        self.startsAtEnd = startsAtEnd
        self.makeModel = makeModel
        self.makeFeed = makeFeed
    }

    var body: some View {
        ZStack {
            MonacoTheme.canvas.ignoresSafeArea()
            if let model, let feed {
                PostContent(model: model, feed: feed, startsAtEnd: startsAtEnd)
            }
        }
        .navigationTitle(PostCopy.title)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .principal) {
                Text(PostCopy.title)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
            }
            if let item = model?.item {
                ToolbarItem(placement: .topBarTrailing) {
                    FeedShareButton(item: item) {
                        Image(monaco: .share)
                            .font(MonacoTheme.Typo.headline)
                            .foregroundStyle(MonacoTheme.primaryText)
                            .frame(minWidth: PostMetrics.round, minHeight: PostMetrics.round)
                    }
                    .frame(minWidth: PostMetrics.round, minHeight: PostMetrics.round)
                    .contentShape(Rectangle())
                    .accessibilityLabel("Share")
                    .accessibilityIdentifier("post-share")
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-item-detail")
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onChange(of: model?.anchorMissingTick) { _, tick in
            if tick != nil, tick != 0 { toasts.current = MonacoToast(message: FeedItemDetailModel.anchorMissing) }
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private func preparedModel() -> FeedItemDetailModel {
        if let model { return model }
        let created = makeModel(environment, itemID, anchorCommentID)
        model = created
        feed = makeFeed(environment)
        return created
    }

    static func liveModel(_ environment: AppEnvironment, itemID: String, anchorCommentID: String?)
        -> FeedItemDetailModel
    {
        FeedItemDetailModel(
            itemID: itemID, anchorCommentID: anchorCommentID, api: environment.api, hints: environment.hints,
            clock: ContinuousClock())
    }

    static func liveFeed(_ environment: AppEnvironment) -> FeedModel {
        FeedModel(api: environment.api, viewerID: nil, hints: environment.hints, clock: ContinuousClock())
    }
}

enum PostCopy {
    static let title = "Post"
}

enum PostMetrics {
    static let round: CGFloat = 44
    static let endGap: CGFloat = 40
}

private struct PostContent: View {
    let model: FeedItemDetailModel
    let feed: FeedModel
    let startsAtEnd: Bool

    @Environment(\.dismiss) private var dismiss
    private let endID = "post-end"

    var body: some View {
        let phase = FeedPostPhase(model)
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    content(phase)
                    Color.clear.frame(height: 0).id(endID)
                }
                .padding(.top, MonacoTheme.Space.s)
            }
            .contentMargins(.bottom, PostMetrics.endGap, for: .scrollContent)
            .scrollDismissesKeyboard(.interactively)
            .environment(\.sectionScrollProxy, proxy)
            .refreshable { await model.refresh() }
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if phase != .unavailable, phase != .failed {
                    CommentComposerBar(model: model.comments, offersPoll: true)
                }
            }
            .onChange(of: model.comments.lastPostedID) { _, id in
                guard let id else { return }
                withAnimation { proxy.scrollTo(id, anchor: .center) }
            }
            .task(id: model.comments.phase) { await scrollToEnd(proxy) }
        }
        .monacoCanvas()
    }

    private func scrollToEnd(_ proxy: ScrollViewProxy) async {
        guard startsAtEnd, model.comments.phase == .loaded else { return }
        for _ in 0..<3 {
            try? await Task.sleep(for: .milliseconds(400))
            proxy.scrollTo(endID, anchor: .bottom)
        }
    }

    @ViewBuilder private func content(_ phase: FeedPostPhase) -> some View {
        switch phase {
        case .loading:
            VStack(alignment: .leading, spacing: 0) {
                PostSkeleton()
                thread
            }
        case .ready(let item):
            VStack(alignment: .leading, spacing: 0) {
                FeedPost(item: item, now: Date(), opensPost: false) {
                    PostReactionsSlot(item: item, model: feed, onReply: { model.reply() })
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                thread
            }
        case .unavailable:
            EmptyState(
                title: "This update isn’t available",
                message: "It may have been removed, or you may no longer have permission to view it.",
                actionTitle: "Back to feed", systemImage: MonacoIcon.lock.symbol, action: { dismiss() }
            )
            .accessibilityIdentifier("feed-item-unavailable")
        case .failed:
            EmptyState(
                title: "Couldn’t load this view",
                message: "Your existing information hasn’t been changed. Try again when your connection is available.",
                actionTitle: "Try again", actionIdentifier: "feed-item-detail-error",
                systemImage: MonacoIcon.offline.symbol, action: { Task { await model.load() } }
            )
        }
    }

    private var thread: some View {
        CommentThreadView(model: model.comments)
            .padding(.top, MonacoTheme.Space.section)
    }
}

private struct PostSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            HStack(spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(
                    width: MonacoAvatarSize.preview.rawValue, height: MonacoAvatarSize.preview.rawValue,
                    radius: MonacoAvatarSize.preview.rawValue / 2)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    SkeletonFraction(SkeletonMetrics.rowSubtitleFraction) { SkeletonBar() }
                    SkeletonFraction(SkeletonMetrics.rowTitleFraction) { SkeletonBar() }
                }
            }
            SkeletonBlock(height: SkeletonMetrics.cardHeight, radius: MonacoTheme.Radius.cardTight)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading the post")
        .accessibilityIdentifier("feed-item-detail-loading")
    }
}
