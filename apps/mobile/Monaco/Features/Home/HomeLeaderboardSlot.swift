import MonacoCore
import SwiftUI

enum HomeLeaderboardSlot: HomeSection {
    static let isLive = true
    static let route = LeaderboardRoute(board: .people, range: .oneMonth)

    static func body(for context: Void) -> some View {
        HomeLeaderboard()
    }
}

enum HomeLeaderboardMetrics {
    static let crownLift: CGFloat = 9
}

private struct HomeLeaderboard: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.homeReads) private var reads
    @State private var model: HomeLeaderboardModel?

    var body: some View {
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: environment.balance.state)
        VStack(alignment: .leading, spacing: 0) {
            switch model?.phase ?? .loading {
            case .loading, .hidden:
                EmptyView()
            case .failed:
                if reads.showsOwnRow(.board) {
                    MonacoErrorRow(thing: "the leaderboard", identifier: "home-leaderboard-error") {
                        Task { await model?.load() }
                    }
                }
            case .ready(let row):
                card(row)
            }
        }
        .homeSlotHidden(!gate.shows { _ in true } || isHidden)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-people-board")
        .task {
            let model = preparedModel()
            refresh?.register("home-people-board") { await model.load() }
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .onChange(of: model?.readStatus, initial: true) { _, status in
            if let status { reads?.report(.board, status) }
        }
    }

    private var isHidden: Bool {
        switch model?.phase ?? .loading {
        case .loading, .hidden: true
        case .failed, .ready: false
        }
    }

    private func card(_ row: HomeLeaderboardRow) -> some View {
        Button {
            environment.navigator.open(HomeLeaderboardSlot.route, in: .home)
        } label: {
            MonacoCard(style: .tight) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    leader(row)
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                        Text(row.title)
                            .font(MonacoTheme.Typo.rowTitle)
                            .foregroundStyle(MonacoTheme.primaryText)
                            .lineLimit(2)
                        Text(row.subtitle)
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.secondaryText)
                            .lineLimit(2)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    if let rate = row.rateText {
                        Text(rate)
                            .font(MonacoTheme.Typo.rowTitle.monospacedDigit())
                            .foregroundStyle(MonacoTheme.primaryText)
                    }
                    MonacoRowChevron()
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(row.spoken)
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("home-leaderboard-row")
    }

    private func leader(_ row: HomeLeaderboardRow) -> some View {
        MonacoAvatar(
            photoURL: row.leaderPictureURL, displayName: row.leaderName, size: .row, seed: row.leaderID
        )
        .overlay(alignment: .top) {
            Image(systemName: "crown.fill")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.goldGlyph)
                .offset(y: -HomeLeaderboardMetrics.crownLift)
                .accessibilityHidden(true)
        }
    }

    private func preparedModel() -> HomeLeaderboardModel {
        if let model { return model }
        let created = HomeLeaderboardModel(api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}
