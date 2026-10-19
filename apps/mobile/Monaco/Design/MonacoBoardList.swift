import MonacoCore
import SwiftUI

struct LeaderboardHost<Content: View>: View {
    let board: LeaderboardBoard
    let refreshKey: String
    var reloadID = 0
    @ViewBuilder let content: (LeaderboardLoader) -> Content

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var loader: LeaderboardLoader?

    var body: some View {
        VStack(spacing: 0) {
            if let loader {
                content(loader)
            } else {
                Color.clear.frame(height: 1)
            }
        }
        .task(id: reloadID) {
            let loader = preparedLoader()
            refresh?.register(refreshKey) { await loader.reload() }
            await loader.load()
            await loader.observe()
        }
        .onScreenVisibilityChange { loader?.setVisible($0) }
        .onChange(of: loader?.toast) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
            loader?.dismissToast()
        }
    }

    private func preparedLoader() -> LeaderboardLoader {
        if let loader { return loader }
        let created = LeaderboardLoader(board: board, api: environment.api, hints: environment.hints)
        loader = created
        return created
    }
}

struct LeaderboardFreshnessText: View {
    let loader: LeaderboardLoader
    let identifier: String
    var font = MonacoTheme.Typo.caption

    var body: some View {
        TimelineView(.periodic(from: .now, by: 30)) { context in
            if let text = loader.freshness(now: context.date) {
                Text(text)
                    .font(font)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier(identifier)
            }
        }
    }
}

enum BoardToolsMetrics {
    static let periodWidth: CGFloat = 86
    static let infoHitHeight: CGFloat = 44
    static let infoHitOverhang: CGFloat = 14
}

struct BoardTools<Category: Hashable, Period: Hashable>: View {
    private let categoryTitle: String
    private let categories: [Category]
    private let categoryLabel: (Category) -> String
    @Binding private var category: Category
    private let periods: [Period]
    private let periodLabel: (Period) -> String
    @Binding private var period: Period
    private let contextText: String
    private let onFilter: () -> Void
    private let onRankInfo: (() -> Void)?

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init(
        categoryTitle: String = "Board", category: Binding<Category>, categories: [Category],
        categoryLabel: @escaping (Category) -> String, period: Binding<Period>, periods: [Period],
        periodLabel: @escaping (Period) -> String, contextText: String, onFilter: @escaping () -> Void,
        onRankInfo: (() -> Void)? = nil
    ) {
        self.categoryTitle = categoryTitle
        _category = category
        self.categories = categories
        self.categoryLabel = categoryLabel
        _period = period
        self.periods = periods
        self.periodLabel = periodLabel
        self.contextText = contextText
        self.onFilter = onFilter
        self.onRankInfo = onRankInfo
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    categoryControl
                    HStack(spacing: MonacoTheme.Space.s) {
                        periodControl.frame(maxWidth: .infinity)
                        filterButton
                    }
                }
            } else {
                HStack(spacing: MonacoTheme.Space.s) {
                    categoryControl
                    periodControl.frame(width: BoardToolsMetrics.periodWidth)
                    filterButton
                }
            }
            contextLine
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.sm)
    }

    @ViewBuilder private var categoryControl: some View {
        if categories.count > 1 {
            MonacoMenuSelect(
                title: categoryTitle, selection: $category, options: categories, label: categoryLabel
            )
            .fillsWidth()
        } else {
            Text(categoryLabel(category))
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var periodControl: some View {
        MonacoMenuSelect(title: "Period", selection: $period, options: periods, label: periodLabel)
            .fillsWidth()
    }

    private var filterButton: some View {
        MonacoGlassButton(
            systemImage: "line.3.horizontal.decrease", accessibilityLabel: "Filter", style: .glass,
            placement: .content, action: onFilter)
    }

    private var contextLine: some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Text(contextText)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            if let onRankInfo {
                Text("·")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
                Button(action: onRankInfo) {
                    HStack(spacing: MonacoTheme.Space.xs) {
                        Text("Rank change")
                        Image(systemName: "info.circle")
                    }
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .frame(minHeight: BoardToolsMetrics.infoHitHeight)
                    .padding(.vertical, -BoardToolsMetrics.infoHitOverhang)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("About rank change")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct LeaderboardBoardList<Empty: View, RowContent: View>: View {
    let loader: LeaderboardLoader
    let skeletonRows: Int
    let failureThing: String
    let identifier: String
    var showsEmpty = true
    var showsFailure = true
    var showsPaging = true
    @ViewBuilder let empty: () -> Empty
    @ViewBuilder let rowContent: (LeaderboardRowView, Bool) -> RowContent

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    init(
        loader: LeaderboardLoader, skeletonRows: Int, failureThing: String, identifier: String,
        showsEmpty: Bool = true, showsFailure: Bool = true, showsPaging: Bool = true,
        @ViewBuilder empty: @escaping () -> Empty,
        @ViewBuilder rowContent: @escaping (LeaderboardRowView, Bool) -> RowContent
    ) {
        self.loader = loader
        self.skeletonRows = skeletonRows
        self.failureThing = failureThing
        self.identifier = identifier
        self.showsEmpty = showsEmpty
        self.showsFailure = showsFailure
        self.showsPaging = showsPaging
        self.empty = empty
        self.rowContent = rowContent
    }

    var body: some View {
        switch loader.phase {
        case .loading:
            BoardRowSkeleton(rows: skeletonRows)
                .accessibilityIdentifier("\(identifier)-loading")
        case .empty:
            if showsEmpty {
                empty()
            }
        case .failed:
            if showsFailure {
                failure
            }
        case .loaded:
            rows
        }
    }

    private var failure: some View {
        MonacoErrorRow(thing: failureThing, identifier: "\(identifier)-error") { loader.retry() }
    }

    private var rows: some View {
        let rows = loader.rows
        let pinned = loader.pinnedMe
        return VStack(spacing: 0) {
            MonacoGroupedList {
                ForEach(rows) { row in
                    rowContent(row, row.id == rows.last?.id && pinned == nil)
                }
                if let pinned {
                    rowContent(pinned, true)
                        .accessibilityIdentifier("\(identifier)-me")
                }
                if loader.isLoadingMore {
                    BoardRowSkeleton(rows: 1)
                }
            }
            if showsPaging, !rows.isEmpty {
                paging(shown: rows.count)
            }
        }
        .opacity(loader.isLoading ? 0.5 : 1)
        .animation(reduceMotion ? nil : .snappy, value: rows.map(\.id))
        .animation(.easeInOut(duration: 0.15), value: loader.isLoading)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(identifier)
    }

    @ViewBuilder
    private func paging(shown: Int) -> some View {
        if loader.hasMore {
            Button("Load more") { Task { await loader.loadMore() } }
                .buttonStyle(.monacoLoadMore)
                .disabled(loader.isLoadingMore)
                .accessibilityIdentifier("\(identifier)-load-more")
        } else {
            Button("All \(shown) shown") {}
                .buttonStyle(.monacoLoadMore)
                .disabled(true)
                .accessibilityIdentifier("\(identifier)-all-shown")
        }
    }
}

extension LeaderboardBoardList where Empty == EmptyState {
    init(
        loader: LeaderboardLoader, skeletonRows: Int, failureThing: String, identifier: String,
        showsEmpty: Bool = true, showsFailure: Bool = true, showsPaging: Bool = true,
        @ViewBuilder rowContent: @escaping (LeaderboardRowView, Bool) -> RowContent
    ) {
        self.init(
            loader: loader, skeletonRows: skeletonRows, failureThing: failureThing, identifier: identifier,
            showsEmpty: showsEmpty, showsFailure: showsFailure, showsPaging: showsPaging,
            empty: { EmptyState(title: "No rankings yet", systemImage: "person.3") }, rowContent: rowContent)
    }
}
