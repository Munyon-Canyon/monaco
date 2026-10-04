import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalProposalsSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalProposalsLive(cabalID: context.cabalID)
    }
}

private struct CabalProposalsLive: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: CabalProposalsModel?

    var body: some View {
        Group {
            if let model {
                CabalProposalsSection(model: model) { route in
                    environment.navigator.open(route, in: environment.navigator.selectedTab)
                }
            } else {
                CabalProposalsSection.loading
            }
        }
        .task {
            guard model == nil else { return }
            model = CabalProposalsModel(
                cabalID: cabalID, repository: ProposalsRepository(api: environment.api), hints: environment.hints)
        }
    }
}

struct CabalProposalsSection: View {
    static let title = "Needs your vote"

    let model: CabalProposalsModel
    let open: (any AppRoute) -> Void

    static var loading: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(title)
            ProposalCardSkeleton()
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    var body: some View {
        content
            .task {
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { model.setVisible($0) }
            .proposalToasts(model.voting)
            .accessibilityIdentifier("cabal-proposals")
    }

    @ViewBuilder
    private var content: some View {
        switch model.state {
        case .idle, .loading:
            Self.loading
        case .failed:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader(Self.title)
                ProposalLoadFailedRow(message: "Couldn't load votes.") {
                    Task { await model.load() }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        case .loaded(.hidden):
            EmptyView()
        case .loaded(.empty):
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                header(count: 0)
                EmptyState(title: "No open votes", message: "Propose the first buy.")
                    .frame(maxWidth: .infinity)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        case .loaded(.cards(let cards, let awaiting)):
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                header(count: awaiting)
                ProposalCardStack(cards: cards, voting: model.voting) { choice, card in
                    Task { await model.vote(choice, on: card) }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }

    private func header(count: Int) -> some View {
        MonacoSectionHeader(Self.title, count: count, trailing: "See all") {
            open(CabalProposalsRoute(cabalID: model.cabalID))
        }
        .accessibilityIdentifier("cabal-proposals-header")
    }
}

#if DEBUG
final class ProposalCardsSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-proposalCardsHarness"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: launchArgument) else { return nil }
        let scenario = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "cabal"
        return AnyView(ProposalCardsHarnessScreen(scenario: scenario))
    }
}

private struct ProposalCardsHarnessScreen: View {
    let scenario: String

    @State private var path: [AnyAppRoute] = []
    @State private var server: ProposalsPreviewServer?
    @State private var cabalModel: CabalProposalsModel?
    @State private var homeModel: PendingVotesModel?

    var body: some View {
        NavigationStack(path: $path) {
            ScrollView {
                VStack(spacing: MonacoTheme.Space.gutter) {
                    if let homeModel {
                        HomePendingVotesSection(model: homeModel) { path.append(AnyAppRoute($0)) }
                    }
                    if let cabalModel {
                        CabalProposalsSection(model: cabalModel) { path.append(AnyAppRoute($0)) }
                    }
                }
                .padding(.vertical, MonacoTheme.Space.m)
            }
            .monacoCanvas()
            .navigationBarTitleDisplayMode(.inline)
            .navigationDestination(for: AnyAppRoute.self) { route in
                let cabalID = Components.Schemas.Cabal.sample(role: "member").id
                if let server, route == AnyAppRoute(CabalProposalsRoute(cabalID: cabalID)) {
                    CabalProposalsListScreen(
                        model: ProposalListModel(
                            cabalID: cabalID, repository: .preview(server), hints: ProposalsPreviewHints()))
                } else if let server, route == AnyAppRoute(PendingVotesRoute()) {
                    PendingVotesScreen(model: PendingVotesModel(repository: .preview(server), limit: nil))
                } else {
                    route.destination()
                }
            }
        }
        .task {
            guard server == nil else { return }
            let server = Self.server(scenario)
            if scenario == "error" { await server.setFailing(true) }
            self.server = server
            let repository = ProposalsRepository.preview(server)
            homeModel = PendingVotesModel(repository: repository, limit: PendingVotesModel.homeLimit)
            cabalModel = CabalProposalsModel(
                cabalID: Components.Schemas.Cabal.sample(role: "member").id, repository: repository,
                hints: ProposalsPreviewHints())
        }
    }

    private static func server(_ scenario: String) -> ProposalsPreviewServer {
        let now = Date()
        let viewer = Components.Schemas.ProposalDetail.sampleVoterIDs[0]
        let jordan = Components.Schemas.ProposalDetail.sampleVoterIDs[1]
        var voted = Components.Schemas.ProposalDetail.sample(
            id: "01890a5d-ac96-774b-bcce-b302099a8064", kind: .sell, ballots: [viewer: .yes, jordan: .yes], now: now)
        voted.createdAt = now.addingTimeInterval(-3 * 3600)
        let open = Components.Schemas.ProposalDetail.sample(now: now)
        let voided = Components.Schemas.ProposalDetail.sample(
            id: "01890a5d-ac96-774b-bcce-b302099a8065", status: .voided, now: now)
        let bought = Components.Schemas.ProposalDetail.sample(
            id: "01890a5d-ac96-774b-bcce-b302099a8066", status: .executed, ballots: [viewer: .yes, jordan: .yes],
            now: now)
        switch scenario {
        case "empty":
            return ProposalsPreviewServer(proposals: [voided])
        case "outsider":
            return ProposalsPreviewServer(cabal: .sampleWithMembers(role: nil), proposals: [open], viewerID: "outsider")
        default:
            return ProposalsPreviewServer(proposals: [open, voted, voided, bought])
        }
    }
}
#endif
