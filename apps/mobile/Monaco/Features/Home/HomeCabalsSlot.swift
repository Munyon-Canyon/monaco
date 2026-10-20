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
    init(item: HomeCabalItem, context: HomeCabalContext?, votesNeededText: String?) {
        let funded = item.isFunded
        let standing = item.standing
        self.init(
            id: item.id, name: item.name,
            memberCountText: item.memberCount.map { CabalCopy.memberCount(Int32(clamping: $0)) }
                ?? context?.memberCountText ?? "Cabal",
            votesNeededText: funded ? votesNeededText : nil,
            shareText: UsdAmountFormatter.format(micros: standing?.valueMicros ?? 0),
            ownershipText: context?.ownershipText ?? (funded ? nil : "0.00%"),
            pnl: funded
                ? standing.map {
                    Figure(amountText: $0.pnl, percentText: $0.returnText, direction: $0.direction.pnlTone)
                } : nil,
            pnlUnavailableReason: funded ? nil : "Not funded", groupReturn: nil,
            tint: MonacoTheme.CabalTint.forGroupId(item.id), pictureURL: item.pictureURL)
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
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.homeReads) private var reads
    @Environment(\.homeVoteNeeds) private var voteNeeds
    @State private var details: HomeCabalContextModel?
    @State private var sharing: String?

    var body: some View {
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: environment.balance.state)
        VStack(alignment: .leading, spacing: 0) {
            if let summary = gate.summary {
                switch HomeCabalList.state(portfolio: summary, memberships: details?.memberships) {
                case .loading:
                    EmptyView()
                case .empty:
                    startWithPeople
                case .cabals(let items):
                    rail(items)
                }
            }
        }
        .homeSlotHidden(!gate.shows { _ in true })
        .task(id: gate.summary) {
            let summary = gate.summary
            refresh?.register("home-cabals") { await sync(summary: summary, refreshing: true) }
            await sync(summary: summary, refreshing: false)
            await details?.observe(hints: environment.hints)
        }
    }

    private func sync(summary: PortfolioSummary?, refreshing: Bool) async {
        let model = details ?? HomeCabalContextModel(api: environment.api)
        details = model
        await model.loadMemberships(refreshing: refreshing)
        guard let summary else { return }
        let items = HomeCabalList.items(portfolio: summary, memberships: model.memberships)
        await model.load(cabalIDs: items.map(\.id))
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

    private func rail(_ items: [HomeCabalItem]) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Your cabals", trailing: "See all") {
                environment.navigator.selectedTab = .cabals
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            CabalCoverRail {
                ForEach(items) { item in
                    CabalCoverCard(
                        model: CabalCoverCardModel(
                            item: item, context: details?.contexts[item.id],
                            votesNeededText: voteNeeds?.text(for: item.id)),
                        isSharing: sharing == item.id,
                        onOpen: { environment.navigator.open(CabalRoute(id: item.id), in: .home) },
                        onShare: {
                            Task {
                                await ShareCardLauncher.shareCabal(
                                    id: item.id, api: environment.api, toasts: toasts, busy: $sharing)
                            }
                        }
                    )
                    .accessibilityElement(children: .contain)
                    .accessibilityIdentifier("cabal-row-\(item.id)")
                }
            }
        }
    }
}
