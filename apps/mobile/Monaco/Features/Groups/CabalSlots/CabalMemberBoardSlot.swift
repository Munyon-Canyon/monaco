import MonacoCore
import SwiftUI

enum CabalMemberBoardSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalMemberBoard(cabalID: context.cabalID)
            .id("cabal-member-board")
    }
}

private struct CabalMemberBoard: View {
    let cabalID: String
    @Environment(\.cabalRetry) private var retry

    var body: some View {
        LeaderboardHost(board: .cabalMembers(id: cabalID), refreshKey: "cabal-member-board", reloadID: retry.tick) {
            loader in
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    MonacoSectionHeader("Leaderboard")
                    LeaderboardFreshnessText(
                        loader: loader, identifier: "cabal-member-board-freshness", font: MonacoTheme.Typo.callout)
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                LeaderboardBoardList(
                    loader: loader, skeletonRows: 3, failureThing: "the leaderboard",
                    identifier: "cabal-member-board", showsEmpty: false,
                    empty: { EmptyView() },
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
                        .accessibilityIdentifier("cabal-member-\(row.id)")
                    })
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityElement(children: .contain)
        }
    }
}
