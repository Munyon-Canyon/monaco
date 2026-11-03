import MonacoCore
import SwiftUI

struct ProposalResultCard<Sides: View>: View {
    let status: ProposalStatus
    let tally: ProposalTally
    let caption: String?
    let accessibilitySummary: String
    @ViewBuilder var sides: () -> Sides
    @Environment(\.proposalDetail) private var model
    @Environment(AppEnvironment.self) private var environment: AppEnvironment?
    @State private var showsVotes = false

    private var seal: (symbol: String, color: Color) {
        switch ProposalResultCopy.seal(status: status) {
        case .passed: ("checkmark.seal.fill", MonacoTheme.profit)
        case .failed: ("xmark.circle", MonacoTheme.secondaryText)
        case .neutral: ("minus.circle", MonacoTheme.secondaryText)
        }
    }

    var body: some View {
        MonacoCard(style: .tight) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HStack(spacing: MonacoTheme.Space.s) {
                    Text(ProposalResultCopy.headline(status: status, tally: tally))
                        .font(MonacoTheme.Typo.display)
                        .foregroundStyle(MonacoTheme.ink)
                    Image(systemName: seal.symbol)
                        .font(MonacoTheme.Typo.headline)
                        .imageScale(.large)
                        .foregroundStyle(seal.color)
                        .accessibilityHidden(true)
                    Spacer(minLength: 0)
                }
                VoteTallyBar(
                    yes: tally.yes, no: tally.no, eligible: tally.voters,
                    winner: ProposalResultCopy.seal(status: status) == .passed ? .yes : nil)
                tallyLine
                if let caption {
                    Text(caption)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.muted)
                        .accessibilityIdentifier("proposal-result-caption")
                }
                if let detail = model?.value, detail.sidesVisible {
                    MonacoRule()
                    ProposalSidesRow(
                        sides: .make(
                            voters: detail.voters, members: model?.members ?? [],
                            viewerID: environment?.viewer?.userID ?? "", sidesVisible: true))
                }
                sides()
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(accessibilitySummary)
        .accessibilityIdentifier("proposal-result")
        .sheet(isPresented: $showsVotes) { ProposalVotesSheet(model: model) }
    }

    @ViewBuilder private var tallyLine: some View {
        let text = HStack(spacing: MonacoTheme.Space.s) {
            Label("\(tally.yes)", systemImage: "checkmark")
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize()
            Text("\(tally.yes + tally.no) of \(tally.voters) voted")
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .frame(maxWidth: .infinity)
            HStack(spacing: MonacoTheme.Space.xs) {
                Text("\(tally.no)")
                Image(systemName: "xmark")
            }
            .font(MonacoTheme.Typo.headline)
            .foregroundStyle(MonacoTheme.primaryText)
            .fixedSize()
        }
        .font(MonacoTheme.Typo.subhead)
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .padding(.vertical, -MonacoTheme.Space.s)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(ProposalResultCopy.tallyLine(tally: tally))
        if model == nil {
            text.accessibilityIdentifier("proposal-result-tally")
        } else {
            Button {
                showsVotes = true
            } label: {
                text.contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityHint("Shows who voted")
            .accessibilityIdentifier("proposal-result-tally")
        }
    }
}
