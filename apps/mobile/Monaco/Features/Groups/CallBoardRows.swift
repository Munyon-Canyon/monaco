import MonacoCore
import MonacoRanking
import SwiftUI

enum CallBoardRoutes {
    static func route(for row: CallBoardRowValue) -> AnyAppRoute {
        row.isCabal
            ? AnyAppRoute(CabalRoute(id: row.id))
            : AnyAppRoute(
                UserProfileRoute(
                    userID: row.id,
                    preview: UserPreview(displayName: row.name, handle: row.handle, photoURL: row.pictureURL)))
    }
}

struct CallBoardRows: View {
    let rows: [CallBoardRowValue]
    var bandID: String?
    var onLastAppear: () -> Void = {}

    var body: some View {
        VStack(spacing: 0) {
            ForEach(rows) { row in
                NavigationLink(value: CallBoardRoutes.route(for: row)) {
                    RankedRow(
                        rank: row.rank.map { .ranked($0, movement: row.movement.signedPlaces) } ?? .unranked,
                        name: row.title, isYou: row.isViewer, subtitle: row.turnout, figure: row.rate,
                        detail: row.record, isYouBand: row.isViewer || row.id == bandID
                    ) {
                        CallBoardAvatar(row: row, size: RankedRowMetrics.avatarSize)
                    }
                }
                .buttonStyle(.monacoRow)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(row.spoken)
                .accessibilityAddTraits(.isButton)
                .accessibilityIdentifier("leaderboard-row-\(row.id)")
                .onAppear { if row.id == rows.last?.id { onLastAppear() } }
            }
        }
    }
}
