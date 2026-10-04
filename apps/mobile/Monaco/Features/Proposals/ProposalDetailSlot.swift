import MonacoCore
import SwiftUI

enum ProposalDetailSlot: ProposalSection {
    static let isLive = true

    static func body(for context: ProposalContext) -> some View {
        ProposalDetailSlotView(proposalID: context.proposalID)
    }
}

private struct ProposalDetailSlotView: View {
    let proposalID: String
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: ProposalDetailModel?

    var body: some View {
        Group {
            if let detail = model?.value {
                ScrollView {
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                        ProposalCard(proposal: detail, asset: model?.asset, members: model?.members ?? []) { choice in
                            Task {
                                await model?.vote(choice)
                                if model?.errorMessage == nil { toasts.show(success: "Vote in") }
                            }
                        }
                        votes(detail)
                        reason(detail)
                        expected(detail)
                    }
                    .padding(MonacoTheme.Space.m)
                }
            } else if model?.errorMessage != nil {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    Text("Couldn't load this proposal.").font(MonacoTheme.Typo.body)
                    Button("Try again") { Task { await model?.load() } }.buttonStyle(.monacoSecondary)
                }
                .padding(MonacoTheme.Space.m)
            } else {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    SkeletonBlock(width: 180, height: 24)
                    SkeletonBlock(width: 120, height: 28)
                    SkeletonBlock(width: 160, height: 12)
                }
                .padding(MonacoTheme.Space.m)
            }
        }
        .task {
            let model = preparedModel()
            await model.load()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private func votes(_ detail: ProposalDetail) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Votes")
            ForEach(detail.voters) { voter in
                let member = model?.members.first { $0.id == voter.id }
                Text("\(member?.name ?? "Member") \(voter.ballot.map { "voted \($0)" } ?? "hasn't voted")")
                    .font(MonacoTheme.Typo.callout)
            }
        }
    }

    @ViewBuilder private func reason(_ detail: ProposalDetail) -> some View {
        if let thesis = detail.summary.thesis, !thesis.isEmpty {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader(detail.summary.kind == "sell" ? "Why sell" : "Why buy")
                Text("“\(thesis)” ").font(MonacoTheme.Typo.callout)
            }
        }
    }

    private func expected(_ detail: ProposalDetail) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Expected")
            if detail.summary.kind == "sell" {
                let micros = detail.summary.quoteOutAmount
                MoneyText(micros: micros, style: .row)
            } else {
                Text("Quote pending").font(MonacoTheme.Typo.callout).foregroundStyle(MonacoTheme.muted)
            }
        }
    }

    private func preparedModel() -> ProposalDetailModel {
        if let model { return model }
        let repository = ProposalsRepository(api: environment.api)
        let created = ProposalDetailModel(id: proposalID, cabalID: "", repository: repository, hints: environment.hints)
        model = created
        return created
    }
}
