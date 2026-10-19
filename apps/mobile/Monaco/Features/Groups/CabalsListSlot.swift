import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalsListSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        MyCabalsList()
    }
}

private struct MyCabalsList: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.cabalsFilter) private var filter
    @State private var model: MonacoCore.CabalsTabModel?
    @State private var details: HomeCabalContextModel?
    @State private var voteCounts: [String: Int] = [:]

    private var cabalIDs: [String] {
        guard case .loaded(let cabals) = model?.state else { return [] }
        return cabals.map(\.id)
    }

    var body: some View {
        MyCabalsContent(
            model: model, standings: model?.standings ?? [:], contexts: details?.contexts ?? [:],
            voteCounts: voteCounts, query: filter.wrappedValue.trimmingCharacters(in: .whitespacesAndNewlines),
            open: open, share: share,
            create: { environment.navigator.open(CreateCabalRoute(), in: .cabals) },
            discover: { environment.navigator.open(DiscoverCabalsRoute(query: $0), in: .cabals) }
        )
        .task {
            let model = preparedModel()
            refresh?.register("cabals-list") { await reload(model) }
            await reload(model)
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .task { await preparedModel().observe(hints: environment.hints) }
        .task(id: cabalIDs) {
            let details = self.details ?? HomeCabalContextModel(api: environment.api)
            self.details = details
            await details.load(cabalIDs: cabalIDs)
        }
        .onChange(of: model?.failureTick) { _, _ in
            guard case .loaded = model?.state, let error = model?.lastError else { return }
            toasts.show(error)
        }
    }

    private func preparedModel() -> MonacoCore.CabalsTabModel {
        if let model { return model }
        let created = MonacoCore.CabalsTabModel(api: environment.api)
        model = created
        return created
    }

    private func reload(_ model: MonacoCore.CabalsTabModel) async {
        await model.load(withStandings: true)
        await loadVotes()
    }

    private func loadVotes() async {
        guard let pending = try? await ProposalsRepository(api: environment.api).pendingVotes() else { return }
        voteCounts = Dictionary(grouping: pending, by: \.cabalID).mapValues(\.count)
    }

    private func open(_ cabal: Components.Schemas.MyCabal) {
        Haptics.selection()
        environment.navigator.open(CabalRoute(id: cabal.id), in: .cabals)
    }

    private func share(_ cabal: Components.Schemas.MyCabal) {
        let standing = CabalInviteStanding(role: cabal.role == "creator" ? .creator : .member)
        environment.navigator.open(InviteMemberRoute(cabalID: cabal.id, standing: standing), in: .cabals)
    }
}

extension CabalCoverCardModel {
    init(
        cabal: Components.Schemas.MyCabal, standing: PortfolioSummary.Row?, context: HomeCabalContext?,
        votesNeeded: Int
    ) {
        let funded = (standing?.valueMicros ?? 0) > 0
        var figure: Figure?
        var unavailable: String? = standing == nil || funded ? "Unavailable" : "Not funded"
        if funded, let standing, standing.returnBps != nil {
            figure = Figure(
                amountText: standing.pnl, percentText: standing.returnText, direction: standing.direction.pnlTone)
            unavailable = nil
        }
        self.init(
            id: cabal.id, name: cabal.name, memberCountText: CabalCopy.memberCount(cabal.memberCount),
            votesNeededText: funded && votesNeeded > 0
                ? (votesNeeded == 1 ? "1 vote needed" : "\(votesNeeded) votes needed") : nil,
            shareText: standing.map { UsdAmountFormatter.format(micros: $0.valueMicros) },
            ownershipText: context?.ownershipText ?? (standing != nil && !funded ? "0.00%" : nil),
            pnl: figure, pnlUnavailableReason: unavailable,
            groupReturn: GroupReturn(periodText: "1M", percentText: "— Return opens soon", direction: .flat),
            tint: MonacoTheme.CabalTint.forGroupId(cabal.id), pictureURL: cabal.pictureUrl)
    }
}

struct MyCabalsContent: View {
    let model: MonacoCore.CabalsTabModel?
    let standings: [String: PortfolioSummary.Row]
    var contexts: [String: HomeCabalContext] = [:]
    var voteCounts: [String: Int] = [:]
    var query = ""
    let open: (Components.Schemas.MyCabal) -> Void
    var share: (Components.Schemas.MyCabal) -> Void = { _ in }
    let create: () -> Void
    var discover: (String) -> Void = { _ in }

    @Environment(\.accountRestricted) private var accountRestricted
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            content
        }
        .padding(.top, MonacoTheme.Space.l)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-list")
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            placeholders
        case .failed:
            EmptyState(
                title: "Couldn't load this view",
                message: "Your existing information hasn't been changed. Try again when your connection is available.",
                actionTitle: "Try again", actionIdentifier: "cabals-list-retry",
                systemImage: MonacoIcon.offline.symbol,
                action: { Task { await model?.load(withStandings: true) } }
            )
        case .loaded(let cabals) where cabals.isEmpty:
            EmptyState(
                title: "Your cabal starts here",
                message: accountRestricted
                    ? "Open a shared invitation to join a cabal." : "Create a cabal or open a shared invitation.",
                actionTitle: accountRestricted ? nil : "Create a cabal",
                systemImage: MonacoIcon.cabals.symbol,
                secondaryActionTitle: accountRestricted ? nil : "Explore cabals",
                action: accountRestricted ? nil : create,
                secondaryAction: accountRestricted ? nil : { discover("") }
            )
            .accessibilityIdentifier("cabals-list-empty")
        case .loaded(let cabals):
            cards(cabals.filter { query.isEmpty || $0.name.localizedCaseInsensitiveContains(query) })
        }
    }

    private func cards(_ shown: [Components.Schemas.MyCabal]) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Your cabals", count: shown.count)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            if shown.isEmpty {
                EmptyState(
                    title: "No matching cabals", message: "Try another name.", systemImage: MonacoIcon.search.symbol
                )
                .accessibilityIdentifier("cabals-list-no-match")
                Button("Search public cabals") { discover(query) }
                    .buttonStyle(.monacoText)
                    .frame(maxWidth: .infinity)
                    .accessibilityIdentifier("cabals-search-public")
            } else {
                LazyVStack(spacing: MonacoTheme.Space.sm) {
                    ForEach(shown, id: \.id) { cabal in
                        card(cabal)
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
    }

    private func card(_ cabal: Components.Schemas.MyCabal) -> some View {
        CabalCoverCard(
            model: CabalCoverCardModel(
                cabal: cabal, standing: standings[cabal.id], context: contexts[cabal.id],
                votesNeeded: voteCounts[cabal.id] ?? 0),
            layout: .list,
            onOpen: { open(cabal) },
            onShare: { share(cabal) }
        )
        .accessibilityValue(Self.spokenBadges(cabal))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-list-card-\(cabal.id)")
    }

    static func spokenBadges(_ cabal: Components.Schemas.MyCabal) -> String {
        [
            cabal.unreadCount > 0 ? CabalCopy.unreadLabel(cabal.unreadCount) : nil,
            CabalCopy.requestBadge(cabal).map(CabalCopy.requestCount),
        ]
        .compactMap { $0 }
        .joined(separator: ", ")
    }

    private var placeholders: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            CabalCoverCardSkeleton(layout: .list)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement()
        .accessibilityLabel("Loading your cabals")
        .accessibilityIdentifier("cabals-list-loading")
    }
}
