import MonacoAPI
import MonacoCore
import SwiftUI

enum HomeCabalsSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeCabals()
    }
}

extension CabalCoverCardModel {
    init(row: PortfolioSummary.Row, context: HomeCabalContext?, votesNeededText: String?) {
        let funded = row.valueMicros > 0
        self.init(
            id: row.id, name: row.name, memberCountText: context?.memberCountText ?? "Cabal",
            votesNeededText: votesNeededText, shareText: UsdAmountFormatter.format(micros: row.valueMicros),
            ownershipText: context?.ownershipText,
            pnl: funded
                ? Figure(amountText: row.pnl, percentText: row.returnText, direction: row.direction.pnlTone) : nil,
            pnlUnavailableReason: funded ? nil : "Not funded", groupReturn: nil,
            tint: MonacoTheme.CabalTint.forGroupId(row.id), pictureURL: row.pictureURL)
    }
}

extension PortfolioSummary.Direction {
    var pnlTone: PnLTone {
        switch self {
        case .up: .profit
        case .down: .loss
        case .flat: .flat
        }
    }
}

private struct HomeCabals: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads
    @Environment(\.homeVoteNeeds) private var voteNeeds
    @State private var details: HomeCabalContextModel?
    @State private var sharing: String?

    var body: some View {
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: environment.balance.state)
        VStack(alignment: .leading, spacing: 0) {
            if let summary = gate.summary {
                if summary.isEmpty {
                    startWithPeople
                } else {
                    rail(summary)
                }
            }
        }
        .homeSlotHidden(!gate.shows { _ in true })
    }

    private var startWithPeople: some View {
        EmptyState(
            title: "Start with your people",
            message: "Create a cabal or open an invitation from a friend.",
            actionTitle: "Create a cabal", systemImage: MonacoIcon.cabals.symbol,
            secondaryActionTitle: "Explore cabals",
            action: { environment.navigator.open(CreateCabalRoute(), in: .home) },
            secondaryAction: { environment.navigator.selectedTab = .cabals }
        )
        .accessibilityIdentifier("home-cabals-empty")
    }

    private func rail(_ summary: PortfolioSummary) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Your cabals", trailing: "See all") {
                environment.navigator.selectedTab = .cabals
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            CabalCoverRail {
                ForEach(summary.rows) { row in
                    CabalCoverCard(
                        model: CabalCoverCardModel(
                            row: row, context: details?.contexts[row.id], votesNeededText: voteNeeds?.text(for: row.id)),
                        isSharing: sharing == row.id,
                        onOpen: { environment.navigator.open(CabalRoute(id: row.id), in: .home) },
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
        .task(id: summary.rows.map(\.id)) {
            let model = details ?? HomeCabalContextModel(api: environment.api)
            details = model
            await model.load(cabalIDs: summary.rows.map(\.id))
        }
    }
}
