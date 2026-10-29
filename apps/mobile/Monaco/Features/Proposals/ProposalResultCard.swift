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
        let text = Text(ProposalResultCopy.tallyLine(tally: tally))
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.muted)
        if model == nil {
            text.accessibilityIdentifier("proposal-result-tally")
        } else {
            Button {
                showsVotes = true
            } label: {
                text.frame(maxWidth: .infinity, alignment: .leading)
            }
            .buttonStyle(.plain)
            .accessibilityHint("Shows who voted")
            .accessibilityIdentifier("proposal-result-tally")
        }
    }
}
