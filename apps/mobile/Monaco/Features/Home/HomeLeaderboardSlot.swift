import MonacoCore
import SwiftUI

enum HomeLeaderboardSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeLeaderboard()
    }
}

enum HomeLeaderboardCategory: Hashable, CaseIterable {
    case cabals
    case people

    var label: String {
        switch self {
        case .cabals: "Cabals"
        case .people: "People"
        }
    }

    var board: LeaderboardBoard {
        switch self {
        case .cabals: .cabals
        case .people: .people
        }
    }

    var contextName: String {
        switch self {
        case .cabals: "Cabal P&L"
        case .people: "Personal P&L"
        }
    }

    func viewAllRoute(range: LeaderboardRange) -> LeaderboardRoute {
        switch self {
        case .cabals: LeaderboardRoute(board: .cabals, range: range)
        case .people: LeaderboardRoute(board: .people, range: range)
        }
    }
}

enum HomeLeaderboardMetrics {
    static let rows = 3
    static let periodWidth: CGFloat = 86
}

private struct HomeLeaderboardHost<Content: View>: View {
    let board: LeaderboardBoard
    let range: LeaderboardRange
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
        .task {
            let loader = preparedLoader()
            refresh?.register("home-people-board") { await loader.reload() }
            await loader.load()
            await loader.observe()
        }
        .onScreenVisibilityChange { loader?.setVisible($0) }
        .onChange(of: loader?.toast) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
            loader?.dismissToast()
        }
        .onChange(of: range) { _, next in loader?.select(range: next) }
    }

    private func preparedLoader() -> LeaderboardLoader {
        if let loader { return loader }
        let created = LeaderboardLoader(board: board, range: range, api: environment.api, hints: environment.hints)
        loader = created
        return created
    }
}

private struct HomeLeaderboard: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads
    @State private var category = HomeLeaderboardCategory.cabals
    @State private var range = LeaderboardRange.oneMonth

    var body: some View {
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: environment.balance.state)
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Leaderboard", trailing: "View all") {
                environment.navigator.open(category.viewAllRoute(range: range), in: .home)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            tools
            HomeLeaderboardHost(board: category.board, range: range) { loader in
                HomeLeaderboardRows(loader: loader, category: category)
            }
            .id(category)
        }
        .homeSlotHidden(!gate.shows { _ in true })
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-people-board")
    }

    private var tools: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoMenuSelect(
                    title: "Leaderboard category", selection: $category, options: HomeLeaderboardCategory.allCases,
                    label: \.label
                )
                .fillsWidth()
                .accessibilityIdentifier("home-leaderboard-category")
                MonacoMenuSelect(
                    title: "Leaderboard period", selection: $range, options: LeaderboardRange.homePeriods,
                    label: \.label
                )
                .frame(width: HomeLeaderboardMetrics.periodWidth)
                .accessibilityIdentifier("home-leaderboard-period")
            }
            Text("\(category.contextName) · \(range.label)")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.xs)
    }
}

private struct HomeLeaderboardRows: View {
    let loader: LeaderboardLoader
    let category: HomeLeaderboardCategory

    @Environment(\.homeReads) private var reads

    var body: some View {
        Group {
            switch loader.phase {
            case .loading:
                BoardRowSkeleton(rows: HomeLeaderboardMetrics.rows)
                    .accessibilityIdentifier("home-leaderboard-loading")
            case .empty:
                empty
            case .failed:
                if reads.showsOwnRow(.board) {
                    MonacoErrorRow(thing: "the leaderboard", identifier: "home-leaderboard-error") {
                        loader.retry()
                    }
                }
            case .loaded:
                rows
            }
        }
        .onChange(of: HomeReadStatus(loader.phase), initial: true) { _, status in
            reads?.report(.board, status)
        }
    }

    private var rows: some View {
        let shown = Array(loader.rows.prefix(HomeLeaderboardMetrics.rows))
        return MonacoGroupedList {
            ForEach(shown) { row in
                NavigationLink(value: route(for: row)) {
                    BoardRow(row: row, isLast: row.id == shown.last?.id, chevron: true) {
                        mark(row)
                    }
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("home-leaderboard-row-\(row.id)")
            }
        }
    }

    private func route(for row: LeaderboardRowView) -> AnyAppRoute {
        switch category {
        case .cabals:
            AnyAppRoute(CabalRoute(id: row.id))
        case .people:
            AnyAppRoute(
                UserProfileRoute(
                    userID: row.id,
                    preview: UserPreview(displayName: row.name, handle: row.handle, photoURL: row.pictureURL)))
        }
    }

    @ViewBuilder private func mark(_ row: LeaderboardRowView) -> some View {
        switch category {
        case .cabals:
            CabalMark(groupId: row.id, name: row.name, size: BoardRowMetrics.avatarSize, pictureUrl: row.pictureURL)
        case .people:
            MonacoAvatar(photoURL: row.pictureURL, displayName: row.name, seed: row.id)
        }
    }

    @ViewBuilder private var empty: some View {
        switch category {
        case .people:
            EmptyState(title: "No investors yet", message: "Fund a cabal to get on the board.")
                .accessibilityIdentifier("home-leaderboard-empty")
        case .cabals:
            EmptyState(
                title: "No cabals ranked yet", message: "Public cabals show up here once they're funded."
            )
            .accessibilityIdentifier("home-leaderboard-empty")
        }
    }
}
