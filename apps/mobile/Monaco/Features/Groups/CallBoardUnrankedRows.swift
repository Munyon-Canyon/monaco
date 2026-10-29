import MonacoRanking
import SwiftUI

struct CallBoardUnrankedRows: View {
    let rows: [CallBoardRowValue]

    var body: some View {
        VStack(spacing: 0) {
            ForEach(rows) { row in
                NavigationLink(value: CallBoardRoutes.route(for: row)) {
                    RankedRow(
                        rank: .unranked, name: row.name, isYou: row.isViewer, subtitle: row.unrankedSubtitle,
                        figure: row.moreFigure, detail: CallBoardCopy.toRank
                    ) {
                        CallBoardAvatar(row: row, size: RankedRowMetrics.avatarSize)
                    }
                }
                .buttonStyle(.monacoRow)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(row.unrankedSpoken)
                .accessibilityAddTraits(.isButton)
                .accessibilityIdentifier("leaderboard-row-\(row.id)")
            }
        }
    }
}
