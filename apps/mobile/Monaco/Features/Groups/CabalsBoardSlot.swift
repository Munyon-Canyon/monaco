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

struct CabalsBoardFilters: Equatable {
    enum Sort: CaseIterable {
        case returnPercent, pnlAmount, name

        var label: String {
            switch self {
            case .returnPercent: "Return %"
            case .pnlAmount: "P&L amount"
            case .name: "Name A–Z"
            }
        }
    }

    enum Membership: CaseIterable {
        case all, acceptingRequests, open, approval

        var label: String {
            switch self {
            case .all: "All"
            case .acceptingRequests: "Accepting requests"
            case .open: "Open"
            case .approval: "Approval"
            }
        }
    }

    enum MinimumMembers: CaseIterable {
        case any, five, ten

        var label: String {
            switch self {
            case .any: "Any cabal size"
            case .five: "5+ members"
            case .ten: "10+ members"
            }
        }
    }

    var sort = Sort.returnPercent
    var membership = Membership.all
    var minimumMembers = MinimumMembers.any

    static let standard = CabalsBoardFilters()

    static var launchDefault: CabalsBoardFilters {
        #if DEBUG
        initial
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
    @State private var applied = CabalsBoardFilters.launchDefault
    @State private var draft = CabalsBoardFilters.launchDefault
    @State private var showsFilters = false
    @State private var showsRankInfo = false

    private var range: Binding<LeaderboardRange> {
        Binding(get: { loader.range }, set: { loader.select(range: $0) })
    }

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
        .onChange(of: loader.range) { _, _ in shown = CabalsBoardMetrics.pageSize }
        .task {
            guard !seededRange else { return }
            seededRange = true
            loader.select(range: .oneMonth)
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
                    style: applied == .standard ? .glass : .selected
                ) {
                    draft = applied
                    showsFilters = true
                }
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
                    Text("Rank change")
                    Image(systemName: MonacoIcon.info.symbol)
                }
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Rank change")
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
            Text("No public rankings for this period.")
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .frame(maxWidth: .infinity)
                .padding(.vertical, MonacoTheme.Space.l)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cabals-board-empty")
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
                            row: row, figure: .pnl, isLast: row.id == visible.last?.id, chevron: true,
                            movement: loader.moves[row.id]
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
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MonacoNotice(.info, message: "Filters open soon. Rankings use return % for now.")
            MonacoMenuSelect(
                title: "Sort by", selection: $draft.sort, options: CabalsBoardFilters.Sort.allCases, label: \.label
            )
            .fillsWidth()
            .disabled(true)
            MonacoMenuSelect(
                title: "Membership", selection: $draft.membership,
                options: CabalsBoardFilters.Membership.allCases, label: \.label
            )
            .fillsWidth()
            .disabled(true)
            MonacoMenuSelect(
                title: "Minimum members", selection: $draft.minimumMembers,
                options: CabalsBoardFilters.MinimumMembers.allCases, label: \.label
            )
            .fillsWidth()
            .disabled(true)
            VStack(spacing: MonacoTheme.Space.s) {
                Button("Apply filters") {
                    applied = draft
                    showsFilters = false
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("cabals-board-filter-apply")
                Button("Reset") { draft = .standard }
                    .buttonStyle(.monacoOutline)
                    .accessibilityIdentifier("cabals-board-filter-reset")
            }
            .padding(.top, MonacoTheme.Space.s)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoSheet(title: "Leaderboard filters", detents: .votes, showsClose: true)
    }

    private var rankInfoSheet: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            Text(
                "The arrow compares this rank with the preceding snapshot using the same period, category and sort. A change from fifth to first is ↑ 4. Missing history shows New, not an invented movement."
            )
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
