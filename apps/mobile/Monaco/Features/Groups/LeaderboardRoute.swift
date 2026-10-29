import MonacoCore
import MonacoRanking
import SwiftUI

nonisolated struct LeaderboardRoute: AppRoute {
    var tab: CallBoardTab = .people
    var period: CallPeriod = .month

    init(tab: CallBoardTab = .people, period: CallPeriod = .month) {
        self.tab = tab
        self.period = period
    }

    init(board: LeaderboardBoardKind, range: LeaderboardRange) {
        tab = board == .cabals ? .cabals : .people
        switch range {
        case .oneWeek: period = .week
        case .oneMonth: period = .month
        default: period = .all
        }
    }

    @MainActor func destination() -> some View {
        LeaderboardView(tab: tab, period: period)
    }
}
