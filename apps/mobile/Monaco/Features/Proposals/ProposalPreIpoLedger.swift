import MonacoCore
import SwiftUI

struct ProposalPreIpoLedger: View {
    let lines: [PreIpoCopy.LedgerLine]
    let eligibility: PreIpoEligibility
    var fillLine = PreIpoCopy.proposalFillLine
    var showsEligibility = true
    var identifier = "proposal-preipo-ledger"

    private var eligible: Bool { eligibility == .eligible }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoCard(style: .inset) {
                VStack(spacing: 0) {
                    ForEach(Array(lines.enumerated()), id: \.offset) { index, line in
                        if index > 0 { MonacoRule() }
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
                        .padding(.vertical, MonacoTheme.Space.sm)
                        .accessibilityElement(children: .combine)
                    }
                }
            }
            if showsEligibility {
                HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                    Image(systemName: (eligible ? MonacoIcon.check : MonacoIcon.info).symbol)
                        .foregroundStyle(eligible ? MonacoTheme.profit : MonacoTheme.secondaryText)
                        .accessibilityHidden(true)
                    Text(PreIpoCopy.proposalEligibilityLine(eligibility, fillLine: fillLine))
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .accessibilityElement(children: .combine)
            }
        }
        .padding(.horizontal, showsEligibility ? MonacoTheme.Space.gutter : 0)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(identifier)
    }
}
