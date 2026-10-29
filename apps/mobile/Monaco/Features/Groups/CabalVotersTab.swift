import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalVotersSlot: CabalSection {
    static let isLive = true

    static func body(for _: CabalContext) -> some View {
        CabalVotersTab()
    }
}

struct CabalVotersTab: View {
    @Environment(\.cabalModel) private var cabalModel

    var body: some View {
        Group {
            if let cabal = cabalModel?.cabal {
                CabalMembersHost(cabal: cabal) { model in
                    CabalVotersTabBody(cabal: cabal, model: model)
                }
            } else if case .failed = cabalModel?.state {
                MonacoErrorRow(thing: "the voters", identifier: "cabal-voters-failed", inset: false) {
                    Task { await cabalModel?.load() }
                }
            } else {
                MonacoRowSkeleton(rows: 4, markShape: .circle, hasTrailing: true)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-voters")
    }
}

private struct CabalVotersTabBody: View {
    let cabal: Components.Schemas.Cabal
    let model: CabalMembersModel

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalModel) private var cabalModel
    @State private var proposal: CabalRuleProposalModel?

    var body: some View {
        let viewerID = environment.viewer?.userID
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            CabalMembersList(model: model) {
                LazyVStack(alignment: .leading, spacing: 0) {
                    if let totals = model.totals {
                        group(
                            CabalMemberCopy.voteTitle(totals: totals), rows: model.voting, viewerID: viewerID)
                        if !model.notVoting.isEmpty {
                            group(
                                CabalMemberCopy.dontVoteTitle(totals: totals), rows: model.notVoting,
                                viewerID: viewerID)
                        }
                    }
                    CabalMembersMore(model: model)
                }
            }
            Text(CabalMemberCopy.votersFooter)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cabal-voters-footer")
        }
        .sheet(item: $proposal) { model in
            CabalRuleProposalSheet(model: model) { outcome in
                proposal = nil
                handle(outcome)
            }
        }
    }

    @ViewBuilder
    private func group(_ title: String, rows: [CabalMemberEntry], viewerID: String?) -> some View {
        MonacoSectionHeader(title)
            .padding(.horizontal, MonacoTheme.Space.gutter)
        ForEach(rows) { entry in
            CabalMemberLink(
                entry: entry, viewerID: viewerID, subtitle: CabalMemberCopy.voterLine(entry.row),
                isLast: entry.id == rows.last?.id
            )
            .contextMenu { actions(entry) }
            .onAppear { Task { await model.loadMore(after: entry) } }
        }
    }

    @ViewBuilder
    private func actions(_ entry: CabalMemberEntry) -> some View {
        if model.canPropose {
            let group = CabalMemberGroup(entry.row)
            Button(CabalMemberCopy.voterAction(group)) {
                propose(.voter(userID: entry.id, canVote: !group.votes))
            }
            .accessibilityIdentifier("cabal-voters-propose-voter")
            Button(CabalMemberCopy.adminAction(group)) {
                propose(.admin(userID: entry.id, isAdmin: group != .admin))
            }
            .accessibilityIdentifier("cabal-voters-propose-admin")
        }
    }

    private func propose(_ change: CabalRuleProposalModel.Change) {
        proposal = CabalRuleProposalModel(cabal: cabal, change: change, api: environment.api)
    }

    private func handle(_ outcome: CabalRuleProposalModel.Outcome) {
        if case .proposed = outcome {
            toasts.show(success: outcome.toast)
        } else {
            toasts.current = MonacoToast(message: outcome.toast)
        }
        Task { await cabalModel?.load() }
    }
}
