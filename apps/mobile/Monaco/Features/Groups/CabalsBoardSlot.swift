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

private struct CabalsBoard: View {
    let loader: LeaderboardLoader

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                MonacoSectionHeader("Top cabals")
                Text("Ranked by return across everyone on Monaco")
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.muted)
                LeaderboardFreshnessText(
                    loader: loader, identifier: "cabals-board-freshness", font: MonacoTheme.Typo.callout)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRangeChips(
                ranges: LeaderboardRange.allCases, selection: loader.range, identifierPrefix: "cabals-board",
                onSelect: { loader.select(range: $0) }
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
            LeaderboardBoardList(
                loader: loader, skeletonRows: 5, failureThing: "cabals", identifier: "cabals-board-list",
                empty: {
                    EmptyState(
                        title: "No cabal has put money in yet", message: "The first one to fund takes the top spot."
                    )
                    .accessibilityIdentifier("cabals-board-empty")
                },
                rowContent: { row, isLast in
                    NavigationLink(value: AnyAppRoute(CabalRoute(id: row.id))) {
                        BoardRow(row: row, figure: .value, isLast: isLast, chevron: true) {
                            CabalMark(groupId: row.id, name: row.name, pictureUrl: row.pictureURL)
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabals-board-row-\(row.id)")
                })
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-board")
    }
}
