import MonacoCore
import SwiftUI

struct ProposalPassedMoment: Equatable, Identifiable {
    let proposalID: String
    var id: String { proposalID }
    let title: String
    let cabalName: String
    let tally: String
    let trade: String
    let calledIt: String
}

extension ProposalPassedMoment {
    @MainActor
    init?(detail: ProposalDetail, model: ProposalDetailModel, viewerID: String?, now: Date = .now) {
        let summary = detail.summary
        guard let swap = summary.swap else { return nil }
        let assetKind = model.asset?.kind ?? .stock
        let isSell = summary.kind == "sell"
        guard
            let trade = ProposalResultCopy.passedSheetTrade(
                swap, symbol: summary.symbol, assetKind: assetKind, isSell: isSell)
        else { return nil }
        let proposer = model.members.first { $0.id == summary.proposerID }?.name ?? "A member"
        proposalID = summary.id
        title = ProposalResultCopy.passedSheetTitle(
            kind: summary.kind, symbol: summary.symbol, assetKind: assetKind,
            amountMicros: isSell ? summary.quoteOutAmount : summary.usdcMicros)
        cabalName = summary.cabalName
        tally = ProposalResultCopy.passedSheetTally(
            tally: summary.tally, closedAt: detail.stats.closedAt ?? swap.filledAt, now: now)
        self.trade = trade
        calledIt = ProposalResultCopy.calledItLine(
            proposerName: CabalInviteCopy.firstName(proposer), isViewer: summary.proposerID == viewerID,
            voted: summary.tally.yes + summary.tally.no, voters: summary.tally.voters)
    }
}

struct ProposalPassedSheet: View {
    let moment: ProposalPassedMoment
    let onShare: () -> Void

    @Environment(\.dismiss) private var dismiss
    @ScaledMetric(relativeTo: .largeTitle) private var sealSize: CGFloat = 56

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            Text(moment.cabalName)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("proposal-passed-cabal")
            MonacoCard {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    HStack(spacing: MonacoTheme.Space.sm) {
                        Image(systemName: "checkmark.seal.fill")
                            .resizable()
                            .scaledToFit()
                            .frame(width: sealSize, height: sealSize)
                            .foregroundStyle(MonacoTheme.brand)
                            .accessibilityHidden(true)
                        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                            Text("Passed")
                                .font(MonacoTheme.Typo.display)
                                .foregroundStyle(MonacoTheme.primaryText)
                            Text(moment.tally)
                                .font(MonacoTheme.Typo.body)
                                .foregroundStyle(MonacoTheme.secondaryText)
                                .accessibilityIdentifier("proposal-passed-tally")
                        }
                    }
                    Text(moment.trade)
                        .font(MonacoTheme.Typo.body)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .accessibilityIdentifier("proposal-passed-trade")
                    Text(moment.calledIt)
                        .font(MonacoTheme.Typo.body)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .accessibilityIdentifier("proposal-passed-called-it")
                }
            }
            Button {
                dismiss()
                onShare()
            } label: {
                Label("Share", systemImage: MonacoIcon.share.symbol)
            }
            .buttonStyle(.monacoSecondary)
            .accessibilityIdentifier("proposal-passed-share")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoSheet(
            title: moment.title, titleIdentifier: "proposal-passed-title", detents: .review, showsClose: true,
            closeIdentifier: "proposal-passed-close"
        )
        .accessibilityIdentifier("proposal-passed-sheet")
        .onAppear { Haptics.success() }
    }
}
