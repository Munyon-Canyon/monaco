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

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            VStack(alignment: .leading, spacing: 2) {
                MonacoSectionHeader("Top investors")
                LeaderboardFreshnessText(loader: loader, identifier: "home-leaderboard-freshness")
            }
            .padding(.horizontal, MonacoTheme.Space.m)
            MonacoRangeChips(
                ranges: LeaderboardRange.allCases, selection: loader.range, identifierPrefix: "home-leaderboard",
                onSelect: { loader.select(range: $0) }
            )
            .padding(.horizontal, MonacoTheme.Space.m)
            LeaderboardBoardList(
                loader: loader, skeletonRows: 5, failureText: "Couldn't load investors.",
                identifier: "home-leaderboard",
                empty: {
                    EmptyState(title: "No investors yet", message: "Fund a cabal to get on the board.")
                        .accessibilityIdentifier("home-leaderboard-empty")
                },
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
                            MonacoAvatar(photoURL: row.pictureURL, displayName: row.name, size: 40, seed: row.id)
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("home-leaderboard-row-\(row.id)")
                })
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-people-board")
    }
}
