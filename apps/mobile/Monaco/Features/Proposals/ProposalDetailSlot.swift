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
    @State private var priorStatus: ProposalStatus?
    @State private var showBurst = false
    @State private var confirmingWithdrawal = false

    var body: some View {
        Group {
            if let detail = model?.value {
                ScrollView {
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                        ProposalCard(proposal: detail.summary, asset: model?.asset, members: model?.members ?? []) {
                            choice in
                            Task {
                                await model?.vote(choice)
                                if model?.errorMessage == nil { toasts.show(success: "Vote in") }
                            }
                        }
                        if model?.canWithdraw == true {
                            Button("Withdraw proposal", role: .destructive) {
                                confirmingWithdrawal = true
                            }
                            .buttonStyle(.monacoDestructive)
                            .accessibilityIdentifier("proposal-withdraw")
                        }
                        votes(detail)
                        reason(detail)
                        expected(detail)
                        status(detail)
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
        .onChange(of: model?.value?.summary.status) { old, new in
            priorStatus = old
            showBurst = old == .open && new == .executed && model?.value?.summary.kind == "buy"
        }
        .confirmationDialog("Withdraw this proposal?", isPresented: $confirmingWithdrawal, titleVisibility: .visible) {
            Button("Withdraw", role: .destructive) {
                Task {
                    let model = preparedModel()
                    await model.withdraw()
                    if model.didWithdraw {
                        toasts.show(success: "Proposal withdrawn.")
                    } else if let message = model.errorMessage {
                        toasts.current = MonacoToast(message: message)
                    }
                }
            }
            .accessibilityIdentifier("proposal-withdraw-confirm")
            Button("Cancel", role: .cancel) {}
                .accessibilityIdentifier("proposal-withdraw-cancel")
        } message: {
            Text("Votes so far are dropped.")
        }
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

    private func status(_ detail: ProposalDetail) -> some View {
        let summary = detail.summary
        let failedSwap = summary.swap?.status == "failed"
        let state = ProposalStepper.state(
            status: summary.status, isSell: summary.kind == "sell", swapFailed: failedSwap)
        return VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Status")
            HStack(spacing: MonacoTheme.Space.s) {
                step(
                    "Voting", active: state == .voting,
                    stamp: summary.createdAt.formatted(date: .abbreviated, time: .shortened))
                step(summary.kind == "sell" ? "Selling" : "Buying", active: state == .trading)
                step("Done", active: state == .done)
            }
            if case .failed(let title) = state {
                Text(summary.swap?.failureMessage ?? summary.statusMessage ?? title).foregroundStyle(MonacoTheme.loss)
            }
            if let swapID = summary.swap?.id {
                NavigationLink(value: AnyAppRoute(TransactionRoute(cabalID: summary.cabalID, transactionID: swapID))) {
                    Label("View transaction", systemImage: "arrow.up.right.square")
                }
            }
            if showBurst { ProposalCoinBurst() }
        }
    }

    private func step(_ title: String, active: Bool, stamp: String? = nil) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title).font(active ? MonacoTheme.Typo.calloutStrong : MonacoTheme.Typo.callout).foregroundStyle(
                active ? MonacoTheme.ink : MonacoTheme.muted)
            if let stamp { Text(stamp).font(MonacoTheme.Typo.micro).foregroundStyle(MonacoTheme.muted) }
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
