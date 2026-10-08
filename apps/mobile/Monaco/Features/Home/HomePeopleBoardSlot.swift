import MonacoCore
import SwiftUI

enum HomePeopleBoardSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        LeaderboardHost(board: .people, refreshKey: "home-people-board") { loader in
            HomePeopleBoard(loader: loader)
        }
    }
}

private struct HomePeopleBoard: View {
    let loader: LeaderboardLoader
    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                MonacoSectionHeader("Top investors")
                LeaderboardFreshnessText(loader: loader, identifier: "home-leaderboard-freshness")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRangeChips(
                ranges: LeaderboardRange.allCases, selection: loader.range, identifierPrefix: "home-leaderboard",
                onSelect: { loader.select(range: $0) }
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoSegmented(
                [LeaderboardFilter.everyone, .friends],
                selection: Binding(get: { loader.filter }, set: { loader.select(filter: $0) })
            ) { $0 == .everyone ? "Everyone" : "Friends" }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("home-leaderboard-filter")
            LeaderboardBoardList(
                loader: loader, skeletonRows: 5, failureThing: "investors",
                identifier: "home-leaderboard",
                empty: { empty },
                rowContent: { row, isLast in
                    NavigationLink(
                        value: AnyAppRoute(
                            UserProfileRoute(
                                userID: row.id,
                                preview: UserPreview(
                                    displayName: row.name, handle: row.handle, photoURL: row.pictureURL)
                            ))
                    ) {
                        BoardRow(row: row, isLast: isLast) {
                            MonacoAvatar(photoURL: row.pictureURL, displayName: row.name, seed: row.id)
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("home-leaderboard-row-\(row.id)")
                })
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-people-board")
    }

    @ViewBuilder private var empty: some View {
        switch loader.filter {
        case .everyone:
            EmptyState(title: "No investors yet", message: "Fund a cabal to get on the board.")
                .accessibilityIdentifier("home-leaderboard-empty")
        case .friends:
            EmptyState(
                title: "Follow people to see how they do.", actionTitle: "Find friends",
                actionIdentifier: "home-leaderboard-find-friends"
            ) {
                environment.navigator.open(FriendsRoute(), in: .home)
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("home-leaderboard-friends-empty")
        }
    }
}
