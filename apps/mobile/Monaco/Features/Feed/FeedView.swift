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
            }
        }
        .navigationTitle(FeedTab.title)
        .navigationBarTitleDisplayMode(.large)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    NavigationLink("Muted", value: AnyAppRoute(FeedMutesRoute()))
                        .accessibilityIdentifier("feed-muted-link")
                } label: {
                    Image(systemName: "ellipsis.circle")
                        .frame(minWidth: 44, minHeight: 44)
                }
                .accessibilityLabel("More")
                .accessibilityIdentifier("feed-menu")
            }
        }
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

private struct FeedScreen: View {
    let model: FeedModel

    @Environment(ToastCenter.self) private var toasts
    @State private var search = ""

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                MonacoSearchField(placeholder: "Search the feed", text: $search)
                    .padding(.horizontal, MonacoTheme.Space.m)
                MonacoChipBar(
                    items: FeedChip.allCases, selected: model.query.chip, title: \.title, identifierPrefix: "feed-chip"
                ) { chip in
                    Task { await model.select(chip) }
                }
                MonacoSegmented(FeedScope.allCases, selection: scope, label: \.title)
                    .padding(.horizontal, MonacoTheme.Space.m)
                    .accessibilityIdentifier("feed-scope")
                content
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .refreshable { await model.refresh() }
        .monacoCanvas()
        .onChange(of: search) { _, text in model.setSearch(text) }
        .onChange(of: model.failureTick) { _, _ in
            guard let error = model.lastError else { return }
            toasts.show(error)
        }
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
            set: { next in Task { await model.select(next) } }
        )
    }

    @ViewBuilder private var content: some View {
        switch model.phase {
        case .loading:
            BoardRowSkeleton(rows: 5)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading the feed")
                .accessibilityIdentifier("feed-loading")
        case .empty(let query):
            EmptyState(title: query.map { "Nothing matches “\($0)”" } ?? "Nothing here yet.")
                .accessibilityIdentifier("feed-empty")
        case .followsNobody:
            VStack(spacing: 0) {
                EmptyState(title: "Follow people to see what they do.")
                NavigationLink("Find friends", value: AnyAppRoute(FriendsRoute()))
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("feed-find-friends")
            }
            .frame(maxWidth: .infinity)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("feed-follow-nobody")
        case .failed:
            MonacoErrorRow(thing: "the feed", identifier: "feed-error") {
                Task { await model.reload() }
            }
        case .loaded:
            cells
        }
    }

    private var cells: some View {
        let items = model.items
        return LazyVStack(spacing: 0) {
            ForEach(Array(items.enumerated()), id: \.element.id) { index, item in
                FeedItemCell(item: item)
                    .contextMenu {
                        ForEach(model.muteOptions(for: item)) { option in
                            Button(option.menuTitle) { mute(option) }
                        }
                    }
                    .overlay(alignment: .bottom) {
                        if index < items.count - 1 { MonacoRule().padding(.leading, MonacoTheme.Space.m) }
                    }
                    .onAppear {
                        guard index >= items.count - 5 else { return }
                        Task { await model.loadMore() }
                    }
            }
            if model.isLoadingMore {
                BoardRowSkeleton(rows: 1)
            }
        }
    }
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
    }
}
#endif
