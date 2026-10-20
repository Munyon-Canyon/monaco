import MonacoAPI
import MonacoCore
import SwiftUI

enum ProfileCabalsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileCabals()
    }
}

extension CabalCoverCardModel {
    init(profileRow row: PortfolioSummary.Row) {
        let funded = row.valueMicros > 0
        let direction: PnLTone =
            switch row.direction {
            case .up: .profit
            case .down: .loss
            case .flat: .flat
            }
        self.init(
            id: row.id, name: row.name, memberCountText: "Cabal",
            shareText: UsdAmountFormatter.format(micros: row.valueMicros), ownershipText: row.slice,
            pnl: funded ? Figure(amountText: row.pnl, percentText: row.returnText, direction: direction) : nil,
            pnlUnavailableReason: funded ? nil : "Not funded",
            tint: MonacoTheme.CabalTint.forGroupId(row.id), pictureURL: row.pictureURL)
    }
}

struct ProfileCabals: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @State private var sharing: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            content
        }
    }

    @ViewBuilder private var content: some View {
        switch portfolio?.state ?? .loading {
        case .idle, .loading:
            section {
                CabalCoverRail {
                    CabalCoverCardSkeleton()
                    CabalCoverCardSkeleton()
                }
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading your cabals")
                .accessibilityIdentifier("profile-cabals-loading")
            }
        case .failed:
            EmptyView()
        case .loaded(let summary) where summary.isEmpty:
            EmptyView()
        case .loaded(let summary):
            section {
                CabalCoverRail {
                    ForEach(summary.rows) { row in
                        CabalCoverCard(
                            model: CabalCoverCardModel(profileRow: row),
                            isSharing: sharing == row.id,
                            onOpen: { environment.navigator.open(CabalRoute(id: row.id), in: .profile) },
                            onShare: {
                                Task {
                                    await ShareCardLauncher.shareCabal(
                                        id: row.id, api: environment.api, toasts: toasts, busy: $sharing)
                                }
                            }
                        )
                        .accessibilityElement(children: .contain)
                        .accessibilityIdentifier("cabal-row-\(row.id)")
                    }
                }
            }
        }
    }

    private func section<Body: View>(@ViewBuilder _ body: () -> Body) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Your cabals", trailing: "See all") {
                environment.navigator.selectedTab = .cabals
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            body()
        }
    }
}
