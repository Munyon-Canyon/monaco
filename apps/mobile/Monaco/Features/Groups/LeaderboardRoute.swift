import MonacoCore
import SwiftUI

nonisolated struct LeaderboardRoute: AppRoute {
    var board: LeaderboardBoardKind = .cabals
    var range: LeaderboardRange = .oneMonth

    @MainActor func destination() -> some View {
        LeaderboardView(board: board, range: range)
    }
}
