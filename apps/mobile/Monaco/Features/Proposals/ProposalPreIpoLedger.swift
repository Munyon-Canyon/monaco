import MonacoCore
import SwiftUI

struct ProposalPreIpoLedger: View {
    let lines: [PreIpoCopy.LedgerLine]
    let eligibility: PreIpoEligibility

    private var eligible: Bool { eligibility == .eligible }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoCard(style: .inset) {
                VStack(spacing: MonacoTheme.Space.s) {
                    ForEach(lines, id: \.label) { line in
                        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                            Text(line.label)
                                .font(MonacoTheme.Typo.subhead)
                                .foregroundStyle(MonacoTheme.secondaryText)
                                .fixedSize(horizontal: false, vertical: true)
                            Spacer(minLength: MonacoTheme.Space.s)
                            Text(line.value)
                                .font(MonacoTheme.Typo.subhead)
                                .monospacedDigit()
                                .foregroundStyle(MonacoTheme.ink)
                        }
                        .accessibilityElement(children: .combine)
                    }
                }
            }
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                Image(systemName: (eligible ? MonacoIcon.check : MonacoIcon.info).symbol)
                    .foregroundStyle(eligible ? MonacoTheme.profit : MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
                Text(PreIpoCopy.proposalEligibilityLine(eligibility))
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .accessibilityElement(children: .combine)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("proposal-preipo-ledger")
    }
}
