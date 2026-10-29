import MonacoCore
import SwiftUI

struct ProposalResultCard<Sides: View>: View {
    let status: ProposalStatus
    let tally: ProposalTally
    let caption: String?
    let accessibilitySummary: String
    @ViewBuilder var sides: () -> Sides

    private var seal: (symbol: String, color: Color) {
        switch ProposalResultCopy.seal(status: status) {
        case .passed: ("checkmark.circle.fill", MonacoTheme.brand)
        case .failed: ("xmark.circle", MonacoTheme.muted)
        case .neutral: ("minus.circle", MonacoTheme.muted)
        }
    }

    var body: some View {
        MonacoCard {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                HStack(spacing: MonacoTheme.Space.s) {
                    Text(ProposalResultCopy.headline(status: status, tally: tally))
                        .font(MonacoTheme.Typo.display)
                        .foregroundStyle(MonacoTheme.ink)
                    Spacer(minLength: MonacoTheme.Space.s)
                    Image(systemName: seal.symbol)
                        .font(.title)
                        .foregroundStyle(seal.color)
                        .accessibilityHidden(true)
                }
                VoteTallyBar(
                    yes: tally.yes, no: tally.no, eligible: tally.voters,
                    winner: ProposalResultCopy.seal(status: status) == .passed ? .yes : nil)
                Text(ProposalResultCopy.tallyLine(tally: tally))
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityIdentifier("proposal-result-tally")
                if let caption {
                    Text(caption)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.muted)
                        .accessibilityIdentifier("proposal-result-caption")
                }
                sides()
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(accessibilitySummary)
        .accessibilityIdentifier("proposal-result")
    }
}
