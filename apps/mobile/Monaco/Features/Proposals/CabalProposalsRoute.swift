import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct CabalProposalsRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        CabalProposalsListLive(cabalID: cabalID)
    }
}

private struct CabalProposalsListLive: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: ProposalListModel?

    var body: some View {
        Group {
            if let model {
                CabalProposalsListScreen(model: model)
            } else {
                Color.clear
            }
        }
        .task {
            guard model == nil else { return }
            model = ProposalListModel(
                cabalID: cabalID, repository: ProposalsRepository(api: environment.api), hints: environment.hints)
        }
    }
}

struct CabalProposalsListScreen: View {
    let model: ProposalListModel

    @State private var filter: ProposalFilter = .open

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                MonacoSegmented(ProposalFilter.allCases, selection: $filter) { $0 == .open ? "Open" : "Closed" }
                    .accessibilityIdentifier("proposal-list-filter")
                list(model.pager(filter))
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle("Votes")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await model.refresh() }
        .task(id: filter) { await model.load(filter) }
        .task { await model.observe() }
        .onScreenVisibilityChange { model.setVisible($0) }
        .proposalToasts(model.voting)
    }

    @ViewBuilder
    private func list(_ pager: CursorPager<ProposalCard>) -> some View {
        switch (pager.phase, pager.items.isEmpty) {
        case (.idle, true), (.loadingFirst, true):
            ProposalCardSkeleton()
        case (.failed, true):
            ProposalLoadFailedRow(message: "Couldn't load votes.") {
                Task { await pager.loadFirst() }
            }
        case (_, true):
            Group {
                if filter == .open {
                    EmptyState(title: "No open votes", message: "Propose the first buy.")
                } else {
                    EmptyState(title: "No closed votes yet")
                }
            }
            .frame(maxWidth: .infinity)
            .padding(.top, MonacoTheme.Space.l)
        case (_, false):
            ProposalCardStack(cards: pager.items, voting: model.voting) { choice, card in
                Task { await model.vote(choice, on: card) }
            }
            if pager.phase == .idle {
                ProgressView()
                    .frame(maxWidth: .infinity)
                    .onAppear { Task { await pager.loadMore() } }
            }
        }
    }
}
