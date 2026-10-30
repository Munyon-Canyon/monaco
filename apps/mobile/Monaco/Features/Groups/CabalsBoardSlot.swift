import MonacoCore
import SwiftUI

enum CabalsBoardSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        LeaderboardHost(board: .cabals, refreshKey: "cabals-board") { loader in
            CabalsBoard(loader: loader)
        }
    }
}

enum CabalsBoardMetrics {
    static let pageSize = 3
    static let loadAreaHeight: CGFloat = 58
}

extension LeaderboardFilters {
    static var cabalsBoardLaunch: LeaderboardFilters {
        #if DEBUG
        cabalsTabSample
        #else
        standard
        #endif
    }
}

private struct CabalsBoard: View {
    let loader: LeaderboardLoader

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var shown = CabalsBoardMetrics.pageSize
    @State private var seededRange = false
    @State private var draft = LeaderboardFilters.standard
    @State private var showsFilters = false
    @State private var showsRankInfo = false

    private var range: Binding<LeaderboardRange> {
        Binding(get: { loader.range }, set: { loader.select(range: $0) })
    }

    private var applied: LeaderboardFilters {
        var filters = LeaderboardFilters.standard
        filters.sort = loader.query.sort
        filters.membership = loader.query.membership
        filters.minimumMembers = loader.query.minimumMembers
        return filters
    }

    private var hasFiltersApplied: Bool { applied != .standard }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoRule()
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.m)
            head
            tools
            contextLine
            ranking
        }
        .padding(.top, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-board")
        .onChange(of: loader.query) { _, _ in shown = CabalsBoardMetrics.pageSize }
        .task {
            guard !seededRange else { return }
            seededRange = true
            loader.select(range: .oneMonth)
            loader.select(filters: .cabalsBoardLaunch)
        }
        .sheet(isPresented: $showsFilters) { filterSheet }
        .sheet(isPresented: $showsRankInfo) { rankInfoSheet }
    }

    private var head: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text("Cabal leaderboard")
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
                .accessibilityAddTraits(.isHeader)
            Spacer(minLength: MonacoTheme.Space.s)
            Text("Public")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var tools: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
        return layout {
            Text("Cabals")
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .frame(maxWidth: .infinity, alignment: .leading)
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoMenuSelect(
                    title: "Leaderboard period", selection: range, options: LeaderboardRange.allCases,
                    label: \.label
                )
                .fillsWidth(dynamicTypeSize.isAccessibilitySize)
                .accessibilityIdentifier("cabals-board-period")
                MonacoGlassButton(
                    systemImage: MonacoIcon.filter.symbol, accessibilityLabel: "Filter and sort cabal leaderboard",
                    style: hasFiltersApplied ? .selected : .glass
                ) {
                    draft = applied
                    showsFilters = true
                }
                .accessibilityValue(hasFiltersApplied ? LeaderboardCopy.filtersOn : "")
                .accessibilityIdentifier("cabals-board-filter")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
    }

    private var contextLine: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text("Cabal P&L · \(loader.range.label)")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            Spacer(minLength: MonacoTheme.Space.s)
            Button {
                showsRankInfo = true
            } label: {
                HStack(spacing: MonacoTheme.Space.xs) {
                    Text("About")
                    Image(systemName: MonacoIcon.info.symbol)
                }
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(LeaderboardCopy.infoLabel)
            .accessibilityIdentifier("cabals-board-rank-info")
        }
        .frame(minHeight: 44)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
    }

    @ViewBuilder private var ranking: some View {
        switch loader.phase {
        case .loading:
            BoardRowSkeleton(rows: CabalsBoardMetrics.pageSize)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading rankings")
                .accessibilityIdentifier("cabals-board-list-loading")
        case .empty:
            VStack(spacing: MonacoTheme.Space.s) {
                Text(
                    hasFiltersApplied
                        ? "\(LeaderboardCopy.filteredEmptyTitle). \(LeaderboardCopy.filteredEmptyMessage)"
                        : "No public rankings for this period."
                )
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("cabals-board-empty")
                if hasFiltersApplied {
                    Button(LeaderboardCopy.resetFilters) { loader.select(filters: .standard) }
                        .buttonStyle(.monacoText)
                        .accessibilityIdentifier("cabals-board-empty-reset")
                }
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, MonacoTheme.Space.l)
            .padding(.horizontal, MonacoTheme.Space.gutter)
        case .failed:
            VStack(spacing: MonacoTheme.Space.s) {
                Text("Rankings couldn't load.")
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                Button("Retry rankings") {
                    shown = CabalsBoardMetrics.pageSize
                    loader.retry()
                }
                .buttonStyle(.monacoText)
                .accessibilityIdentifier("cabals-board-list-error")
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, MonacoTheme.Space.l)
            .padding(.horizontal, MonacoTheme.Space.gutter)
        case .loaded:
            rows
        }
    }

    private var rows: some View {
        let loaded = loader.rows
        let visible = Array(loaded.prefix(shown))
        return VStack(spacing: 0) {
            MonacoGroupedList {
                ForEach(visible) { row in
                    NavigationLink(value: AnyAppRoute(CabalRoute(id: row.id))) {
                        BoardRow(
                            row: row, figure: .pnl, isLast: row.id == visible.last?.id, chevron: true
                        ) {
                            CabalMark(groupId: row.id, name: row.name, pictureUrl: row.pictureURL)
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabals-board-row-\(row.id)")
                }
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabals-board-list")
            loadArea(loaded.count)
        }
        .opacity(loader.isLoading ? 0.5 : 1)
    }

    @ViewBuilder private func loadArea(_ loadedCount: Int) -> some View {
        Group {
            if loader.isLoadingMore {
                Button("Loading more rankings…") {}
                    .buttonStyle(.monacoText)
                    .disabled(true)
            } else if shown < loadedCount || loader.hasMore {
                Button("Load more rankings") { Task { await reveal() } }
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier("cabals-board-load-more")
            } else {
                Text("All \(loadedCount) eligible cabals shown")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("cabals-board-all-shown")
            }
        }
        .frame(maxWidth: .infinity, minHeight: CabalsBoardMetrics.loadAreaHeight)
    }

    private func reveal() async {
        let target = shown + CabalsBoardMetrics.pageSize
        if target > loader.rows.count, loader.hasMore {
            await loader.loadMore()
        }
        let before = min(shown, loader.rows.count)
        shown = target
        let added = min(target, loader.rows.count) - before
        if added > 0 {
            AccessibilityNotification.Announcement("\(added) more rankings loaded").post()
        }
    }

    private var filterSheet: some View {
        CabalsBoardFilterSheet(
            draft: $draft,
            apply: {
                loader.select(filters: draft)
                showsFilters = false
            },
            reset: {
                draft = .standard
                loader.select(filters: .standard)
                showsFilters = false
            })
    }

    private var rankInfoSheet: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            Text(
                "Dollar P&L and percentage return share the selected period. Deposits are not gains. Only permitted public performance is ranked."
            )
        }
        .font(MonacoTheme.Typo.subhead)
        .foregroundStyle(MonacoTheme.bodyText)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoSheet(title: "About this ranking", detents: .votes, showsClose: true)
    }
}

private struct CabalsBoardFilterSheet: View {
    @Binding var draft: LeaderboardFilters
    let apply: () -> Void
    let reset: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MonacoMenuSelect(
                title: LeaderboardCopy.sortTitle, selection: $draft.sort, options: LeaderboardSort.allCases,
                label: \.label
            )
            .fillsWidth()
            MonacoMenuSelect(
                title: LeaderboardCopy.membershipTitle, selection: $draft.membership,
                options: LeaderboardMembership.allCases, label: \.label
            )
            .fillsWidth()
            MonacoMenuSelect(
                title: LeaderboardCopy.minimumMembersTitle, selection: $draft.minimumMembers,
                options: LeaderboardMinimumMembers.allCases, label: \.label
            )
            .fillsWidth()
            VStack(spacing: MonacoTheme.Space.s) {
                Button(LeaderboardCopy.apply, action: apply)
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("cabals-board-filter-apply")
                Button(LeaderboardCopy.reset, action: reset)
                    .buttonStyle(.monacoOutline)
                    .accessibilityIdentifier("cabals-board-filter-reset")
            }
            .padding(.top, MonacoTheme.Space.s)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoSheet(title: LeaderboardCopy.filterSheetTitle, detents: .votes, showsClose: true)
    }
}
