import MonacoCore
import SwiftUI

enum LeaderboardMetrics {
    static let skeletonRows = 6
    static let footerHeight: CGFloat = 48
    static let filterDot: CGFloat = 6
}

struct LeaderboardView: View {
    let board: LeaderboardBoardKind
    let range: LeaderboardRange
    var filters = LeaderboardFilters.standard
    var fixture: LeaderboardFixture?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var model: LeaderboardPageModel?
    @State private var showsFilters = false
    @State private var showsAbout = false

    var body: some View {
        Group {
            if let model {
                page(model)
            } else {
                Color.clear
            }
        }
        .monacoCanvas()
        .navigationTitle(LeaderboardCopy.title)
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("leaderboard-page")
        .onAppear { model = model ?? makeModel() }
        .task(id: model?.category) { await model?.activate() }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .onChange(of: model?.toast) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
            model?.dismissToast()
        }
        .sheet(isPresented: $showsAbout) { LeaderboardAboutSheet() }
    }

    private func makeModel() -> LeaderboardPageModel {
        let api = environment.api
        let hints = environment.hints
        return LeaderboardPageModel(board: board, range: range, filters: filters, fixture: fixture) {
            kind, range, filters in
            LeaderboardLoader(
                board: kind, range: range, filter: kind == .people ? filters.scope.filter : .everyone,
                filters: filters, api: api, hints: hints)
        }
    }

    private func page(_ model: LeaderboardPageModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                if model.pageState == .failed {
                    failure(model)
                } else {
                    tools(model)
                    contextLine(model)
                    content(model)
                }
            }
        }
        .refreshable { await model.reload() }
        .sheet(isPresented: $showsFilters, onDismiss: model.discardDraft) {
            LeaderboardFilterSheet(model: model) { showsFilters = false }
        }
    }

    private func tools(_ model: LeaderboardPageModel) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
        return layout {
            MonacoMenuSelect(
                title: LeaderboardCopy.categoryTitle, selection: categoryBinding(model),
                options: LeaderboardBoardKind.allCases, label: \.label
            )
            .fillsWidth()
            .accessibilityIdentifier("leaderboard-category")
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoMenuSelect(
                    title: LeaderboardCopy.periodTitle, selection: rangeBinding(model),
                    options: LeaderboardRange.allCases, label: \.label
                )
                .fillsWidth(dynamicTypeSize.isAccessibilitySize)
                .accessibilityIdentifier("leaderboard-period")
                filterButton(model)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
    }

    private func filterButton(_ model: LeaderboardPageModel) -> some View {
        MonacoGlassButton(
            systemImage: MonacoIcon.filter.symbol, accessibilityLabel: LeaderboardCopy.filterLabel,
            style: model.hasFiltersApplied ? .selected : .glass
        ) {
            model.beginEditingFilters()
            showsFilters = true
        }
        .overlay(alignment: .topTrailing) {
            if model.hasFiltersApplied {
                Circle()
                    .fill(MonacoTheme.brand)
                    .frame(width: LeaderboardMetrics.filterDot, height: LeaderboardMetrics.filterDot)
                    .accessibilityHidden(true)
            }
        }
        .accessibilityValue(model.hasFiltersApplied ? LeaderboardCopy.filtersOn : "")
        .accessibilityIdentifier("leaderboard-filter")
    }

    private func contextLine(_ model: LeaderboardPageModel) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 0))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
        return layout {
            Text(model.contextLine)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(maxWidth: .infinity, alignment: .leading)
            Button {
                showsAbout = true
            } label: {
                Image(systemName: MonacoIcon.info.symbol)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .frame(minWidth: 44, minHeight: 44)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(LeaderboardCopy.infoLabel)
            .accessibilityIdentifier("leaderboard-info")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, MonacoTheme.Space.sm)
    }

    private func categoryBinding(_ model: LeaderboardPageModel) -> Binding<LeaderboardBoardKind> {
        Binding(get: { model.category }, set: { model.select(category: $0) })
    }

    private func rangeBinding(_ model: LeaderboardPageModel) -> Binding<LeaderboardRange> {
        Binding(get: { model.range }, set: { model.select(range: $0) })
    }

}

extension LeaderboardView {
    @ViewBuilder private func content(_ model: LeaderboardPageModel) -> some View {
        switch model.pageState {
        case .loading:
            BoardRowSkeleton(rows: LeaderboardMetrics.skeletonRows)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading rankings")
        case .rows:
            rows(model)
        case .empty:
            empty(model)
        case .filteredEmpty:
            filteredEmpty(model)
        case .stale:
            MonacoNotice(.warn, message: LeaderboardCopy.stale)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("leaderboard-stale")
        case .failed:
            EmptyView()
        }
    }

    private func failure(_ model: LeaderboardPageModel) -> some View {
        EmptyState(
            title: LeaderboardCopy.errorTitle, message: LeaderboardCopy.errorMessage,
            actionTitle: LeaderboardCopy.tryAgain, actionIdentifier: "leaderboard-error",
            systemImage: "wifi.exclamationmark", action: { model.retry() })
    }

    @ViewBuilder private func empty(_ model: LeaderboardPageModel) -> some View {
        if model.followingIsEmpty {
            EmptyState(
                title: LeaderboardCopy.followingEmptyTitle, actionTitle: LeaderboardCopy.findFriends,
                systemImage: "chart.line.uptrend.xyaxis"
            ) {
                environment.navigator.open(FriendsRoute(), in: environment.navigator.selectedTab)
            }
            .accessibilityIdentifier("leaderboard-empty")
        } else {
            EmptyState(
                title: LeaderboardCopy.emptyTitle, message: LeaderboardCopy.emptyMessage,
                systemImage: "chart.line.uptrend.xyaxis"
            )
            .accessibilityIdentifier("leaderboard-empty")
        }
    }

    private func filteredEmpty(_ model: LeaderboardPageModel) -> some View {
        VStack(spacing: 0) {
            EmptyState(
                title: LeaderboardCopy.filteredEmptyTitle, message: LeaderboardCopy.filteredEmptyMessage,
                systemImage: MonacoIcon.filter.symbol)
            Button(LeaderboardCopy.resetFilters) { model.resetFilters() }
                .buttonStyle(.monacoOutline)
                .padding(.horizontal, MonacoTheme.Space.gutter)
        }
        .accessibilityIdentifier("leaderboard-empty")
    }

}

extension LeaderboardView {
    private func rows(_ model: LeaderboardPageModel) -> some View {
        let entries = model.entries
        let pinned = model.pinnedMe
        return VStack(spacing: 0) {
            MonacoGroupedList {
                ForEach(entries) { entry in
                    rowLink(entry, isLast: entry.id == entries.last?.id && pinned == nil, model: model)
                }
                if let pinned {
                    rowLink(
                        LeaderboardPageRow(row: pinned, movement: nil, comparesMovement: true, memberCount: nil),
                        isLast: true, model: model)
                }
                if model.isLoadingMore {
                    BoardRowSkeleton(rows: 1)
                }
            }
            footer(model, shown: entries.count)
        }
        .accessibilityElement(children: .contain)
    }

    private func rowLink(_ entry: LeaderboardPageRow, isLast: Bool, model: LeaderboardPageModel) -> some View {
        let row = entry.row
        let route =
            model.category == .cabals
            ? AnyAppRoute(CabalRoute(id: row.id))
            : AnyAppRoute(
                UserProfileRoute(
                    userID: row.id,
                    preview: UserPreview(displayName: row.name, handle: row.handle, photoURL: row.pictureURL)))
        return NavigationLink(value: route) {
            BoardRow(
                row: row, figure: .pnl, isLast: isLast, chevron: true, movement: entry.glyphMovement,
                subtitle: entry.subtitle
            ) {
                avatar(row, model.category)
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spoken(entry, board: model.category))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("leaderboard-row-\(row.id)")
        .onAppear {
            guard row.id == model.entries.last?.id, model.hasMore else { return }
            Task { await model.loadMore() }
        }
    }

    @ViewBuilder private func avatar(_ row: LeaderboardRowView, _ kind: LeaderboardBoardKind) -> some View {
        switch kind {
        case .cabals:
            CabalMark(groupId: row.id, name: row.name, size: BoardRowMetrics.avatarSize, pictureUrl: row.pictureURL)
        case .people:
            MonacoAvatar(photoURL: row.pictureURL, displayName: row.name, seed: row.id)
        }
    }

    @ViewBuilder private func footer(_ model: LeaderboardPageModel, shown: Int) -> some View {
        if !model.isLoadingMore, !model.hasMore, shown > 0 {
            Text(LeaderboardCopy.allShown(board: model.category, count: shown))
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(maxWidth: .infinity, minHeight: LeaderboardMetrics.footerHeight)
        }
    }

}

extension LeaderboardView {
    static func spoken(_ entry: LeaderboardPageRow, board: LeaderboardBoardKind) -> String {
        let row = entry.row
        var parts = ["Rank \(row.rank)"]
        if !entry.comparesMovement {
            parts.append(LeaderboardCopy.movementNotCompared)
        } else if let movement = entry.movement {
            parts.append(LeaderboardCopy.spoken(movement))
        }
        parts.append(row.name)
        if row.isViewer { parts.append(BoardRow<EmptyView>.viewerLabel) }
        if let subtitle = entry.subtitle {
            parts.append(subtitle)
        } else if board == .people, let handle = row.handle {
            parts.append("@\(handle)")
        }
        if let bps = row.returnBps {
            parts.append(PnLSpeech.percent(PercentFormatter.format(basisPoints: bps, signed: true)))
        } else {
            parts.append("no return yet")
        }
        parts.append(PnLSpeech.dollars(UsdAmountFormatter.format(signedMicros: row.pnlMicros)))
        if row.pricesDelayed { parts.append("prices delayed") }
        return parts.joined(separator: ", ")
    }
}
