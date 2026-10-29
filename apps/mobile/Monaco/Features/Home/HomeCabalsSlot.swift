import MonacoCore
import SwiftUI

enum HomeCabalsSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeCabals()
    }
}

private struct HomeCabals: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.homeReads) private var reads
    @Environment(\.homeVoteNeeds) private var voteNeeds
    @State private var details: HomeCabalContextModel?
    @State private var answering: HomeInviteRow?

    var body: some View {
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: environment.balance.state)
        VStack(alignment: .leading, spacing: 0) {
            if let summary = gate.summary {
                switch HomeCabalList.state(portfolio: summary, invites: details?.invites) {
                case .loading:
                    EmptyView()
                case .empty:
                    startWithPeople
                case .cabals(let content):
                    section(content)
                }
            }
        }
        .homeSlotHidden(!gate.shows { _ in true })
        .sheet(item: $answering) { row in
            CabalInviteAnswerSheet(invite: row.invite) {
                answering = nil
                Task {
                    await details?.load(refreshing: true)
                    await portfolio?.load()
                }
            }
        }
        .task {
            let model = preparedDetails()
            refresh?.register("home-cabals") { await model.load(refreshing: true) }
            await model.load()
            await model.observe(hints: environment.hints)
        }
    }

    private func preparedDetails() -> HomeCabalContextModel {
        if let details { return details }
        let created = HomeCabalContextModel(api: environment.api)
        details = created
        return created
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

    private func section(_ content: HomeCabalsContent) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            header(content)
            MonacoGroupedList {
                ForEach(content.shown) { row in
                    CabalPortfolioRow(
                        row: row, votes: voteNeeds?.byCabal[row.id] ?? 0, votesText: voteNeeds?.text(for: row.id),
                        isLast: row.id == content.shown.last?.id && content.invites.isEmpty)
                }
                ForEach(content.invites) { invite in
                    HomeInviteCabalRow(row: invite, isLast: invite.id == content.invites.last?.id) {
                        answering = invite
                    }
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-cabals")
    }

    private func header(_ content: HomeCabalsContent) -> some View {
        MonacoSectionHeader("Your cabals", trailing: "See all", actionIdentifier: "home-cabals-see-all") {
            environment.navigator.selectedTab = .cabals
        }
        .countText(content.countText)
        .padding(.horizontal, MonacoTheme.Space.gutter)
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
