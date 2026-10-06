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
                FeedChipBar(selected: model.query.chip) { chip in
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
            EmptyState(title: "Couldn't load the feed.", actionTitle: "Try again") {
                Task { await model.reload() }
            }
            .accessibilityIdentifier("feed-error")
        case .loaded:
            cells
        }
    }

    private var cells: some View {
        let items = model.items
        return LazyVStack(spacing: 0) {
            ForEach(Array(items.enumerated()), id: \.element.id) { index, item in
                FeedItemCell(item: item)
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

private struct FeedChipBar: View {
    let selected: FeedChip
    let select: (FeedChip) -> Void

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: MonacoTheme.Space.s) {
                ForEach(FeedChip.allCases, id: \.self) { chip in
                    let isSelected = chip == selected
                    Button {
                        guard !isSelected else { return }
                        Haptics.selection()
                        select(chip)
                    } label: {
                        Text(chip.title)
                            .font(MonacoTheme.Typo.calloutStrong)
                            .foregroundStyle(isSelected ? MonacoTheme.primaryButtonLabel : MonacoTheme.ink)
                            .padding(.horizontal, MonacoTheme.Space.m)
                            .frame(minHeight: 36)
                            .background(
                                isSelected ? MonacoTheme.primaryButtonFill : MonacoTheme.surfaceSunken, in: Capsule()
                            )
                            .frame(minHeight: 44)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(isSelected ? [.isSelected] : [])
                    .accessibilityIdentifier("feed-chip-\(chip.title)")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.m)
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
