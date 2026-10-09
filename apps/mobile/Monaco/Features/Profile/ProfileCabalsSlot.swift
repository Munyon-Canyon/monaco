import MonacoAPI
import MonacoCore
import SwiftUI

enum ProfileCabalsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileCabals()
    }
}

struct ProfileCabals: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Your cabals")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            content
        }
    }

    @ViewBuilder private var content: some View {
        switch portfolio?.state ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 3, markShape: .tile)
                .accessibilityElement()
                .accessibilityLabel("Loading your cabals")
                .accessibilityIdentifier("profile-cabals-loading")
        case .failed:
            EmptyView()
        case .loaded(let summary) where summary.isEmpty:
            EmptyState(
                title: "No money in a cabal yet",
                message: "Fund a cabal to see it here.",
                actionTitle: "Browse cabals"
            ) {
                environment.navigator.selectedTab = .cabals
            }
            .accessibilityIdentifier("profile-cabals-empty")
        case .loaded(let summary):
            MonacoGroupedList {
                ForEach(summary.rows) { row in
                    CabalPortfolioRow(row: row, isLast: row.id == summary.rows.last?.id)
                }
            }
        }
    }
}
