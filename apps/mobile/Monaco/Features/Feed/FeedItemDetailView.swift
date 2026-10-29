import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct FeedItemRoute: AppRoute {
    let itemID: String

    @MainActor func destination() -> FeedItemDetailView {
        FeedItemDetailView(itemID: itemID)
    }
}

struct FeedItemDetailView: View {
    let itemID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: FeedItemDetailModel?
    private let makeModel: @MainActor (AppEnvironment, String) -> FeedItemDetailModel

    init(
        itemID: String,
        makeModel: @escaping @MainActor (AppEnvironment, String) -> FeedItemDetailModel =
            FeedItemDetailView.liveModel
    ) {
        self.itemID = itemID
        self.makeModel = makeModel
    }

    var body: some View {
        ZStack {
            MonacoTheme.canvas.ignoresSafeArea()
            if let model {
                FeedItemDetailContent(model: model)
            }
        }
        .navigationTitle("Post & comments")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-item-detail")
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private func preparedModel() -> FeedItemDetailModel {
        if let model { return model }
        let created = makeModel(environment, itemID)
        model = created
        return created
    }

    static func liveModel(_ environment: AppEnvironment, itemID: String) -> FeedItemDetailModel {
        FeedItemDetailModel(itemID: itemID, api: environment.api, hints: environment.hints, clock: ContinuousClock())
    }
}

private struct FeedItemDetailContent: View {
    let model: FeedItemDetailModel

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        let phase = FeedPostPhase(model)
        ScrollViewReader { proxy in
            ScrollView {
                content(phase)
                    .padding(.vertical, MonacoTheme.Space.s)
            }
            .scrollDismissesKeyboard(.interactively)
            .refreshable { await model.refresh() }
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if phase != .unavailable, phase != .failed {
                    CommentComposerBar(model: model.comments)
                }
            }
            .onChange(of: model.comments.lastPostedID) { _, id in
                guard let id else { return }
                withAnimation { proxy.scrollTo(id, anchor: .center) }
            }
        }
        .monacoCanvas()
    }

    @ViewBuilder private func content(_ phase: FeedPostPhase) -> some View {
        switch phase {
        case .loading:
            VStack(alignment: .leading, spacing: 0) {
                FeedPostSkeleton()
                thread
            }
        case .ready(let item):
            VStack(alignment: .leading, spacing: 0) {
                FeedDetailHeader(item: item)
                FeedPostSummary(item: item)
                    .padding(.top, MonacoTheme.Space.m)
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

private struct FeedPostSkeleton: View {
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

private struct FeedDetailHeader: View {
    let item: Components.Schemas.FeedItem

    private var name: String {
        item.actorName.flatMap { $0.isEmpty ? nil : $0 } ?? CommentsCopy.unknownAuthor
    }

    private var byline: String {
        "\(RelativeTimeFormatter.label(date: item.createdAt, now: Date())) · \(item.cabalName ?? "Public")"
    }

    var body: some View {
        if let actorID = item.actorId {
            ViewThatFits(in: .horizontal) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    author(actorID)
                    Spacer(minLength: MonacoTheme.Space.s)
                    returnPill
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    author(actorID)
                    returnPill
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.s)
        } else {
            MonacoSectionHeader("Market update")
                .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }

    private func author(_ actorID: String) -> some View {
        NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: actorID))) {
            HStack(spacing: MonacoTheme.Space.sm) {
                MonacoAvatar(photoURL: item.actorPhotoUrl, displayName: name, size: .preview, seed: actorID)
                VStack(alignment: .leading, spacing: 0) {
                    Text(name)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .monacoPrivate()
                    Text(byline)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .monacoPrivate()
                }
            }
            .frame(minHeight: FeedCardMetrics.authorRow)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Open \(name) profile, \(byline)")
        .accessibilityIdentifier("feed-post-author")
    }

    @ViewBuilder private var returnPill: some View {
        if let facts = FeedReturnFacts(FeedFigureFormat.authorReturn(item)) {
            FeedReturnPill(facts: facts)
        }
    }
}

private struct FeedPostSummary: View {
    let item: Components.Schemas.FeedItem

    private var summaryText: String? {
        [item.body, item.detail, item.status].compactMap { $0 }.first { !$0.isEmpty }
    }

    var summary: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(item.title)
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
            if let text = summaryText {
                Text(text)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.bodyText)
                    .lineSpacing(MonacoTheme.Space.xs)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, MonacoTheme.Space.sm)
            }
            if let symbol = item.symbol, !symbol.isEmpty {
                assetRow(symbol)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.xs)
        .padding(.vertical, MonacoTheme.Space.s)
    }

    var body: some View {
        MonacoCard(style: .tight) { summary }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("feed-post-summary")
    }

    private func assetRow(_ symbol: String) -> some View {
        NavigationLink(value: item.opensRoute) {
            MonacoRow(
                title: item.assetName ?? AssetSymbolFormatter.display(symbol), titleFont: MonacoTheme.Typo.headline,
                subtitle: item.cabalName ?? "Token market", chevron: true, isLast: true,
                leading: { StockMark(symbol: symbol, size: .row, logoURL: item.assetLogoURL) },
                trailing: {
                    if let amount = FeedFigureFormat.amount(item) {
                        Text(amount)
                            .font(MonacoTheme.Typo.subhead)
                            .foregroundStyle(MonacoTheme.primaryText)
                            .monacoPrivate()
                    }
                })
        }
        .buttonStyle(.monacoRow)
        .monacoBleed()
        .padding(.top, MonacoTheme.Space.sm)
    }
}

#if DEBUG
final class FeedPostSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-feedPostHarness") else { return nil }
        let raw = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "ready"
        let acts = arguments.contains(SampleHarnessRegistry.actArgument)
        return AnyView(
            FeedPostHarnessScreen(mode: FeedItemDetailModel.PreviewMode(rawValue: raw) ?? .ready, acts: acts))
    }
}

private struct FeedPostHarnessScreen: View {
    let mode: FeedItemDetailModel.PreviewMode
    let acts: Bool

    @State private var model: FeedItemDetailModel
    @State private var path: [AnyAppRoute]

    init(mode: FeedItemDetailModel.PreviewMode, acts: Bool) {
        self.mode = mode
        self.acts = acts
        _model = State(initialValue: .preview(mode, clock: ContinuousClock()))
        _path = State(initialValue: [AnyAppRoute(FeedPostSampleRoute())])
    }

    var body: some View {
        NavigationStack(path: $path) {
            FeedView(makeModel: { _ in .preview(.items, clock: ContinuousClock()) })
                .navigationDestination(for: AnyAppRoute.self) { route in
                    if route.is(FeedPostSampleRoute.self) {
                        FeedItemDetailView(itemID: FeedItemDetailModel.previewItemID, makeModel: { _, _ in model })
                    } else {
                        route.destination()
                    }
                }
        }
        .task { await act() }
    }

    private func act() async {
        guard acts else { return }
        await model.load()
        try? await Task.sleep(for: .milliseconds(500))
        switch mode {
        case .replying:
            if let row = model.comments.rows.first { model.comments.presentReplySheet(.comment(row)) }
        case .sending, .failed:
            _ = await model.comments.post(CommentDraft(text: "Count me in if we cap it at $25."))
        case .ready, .empty, .unavailable:
            break
        }
    }
}

nonisolated private struct FeedPostSampleRoute: AppRoute {
    @MainActor func destination() -> EmptyView { EmptyView() }
}
#endif
