import MonacoAPI
import MonacoCore
import SwiftUI

struct ProposePotTotalRow: View {
    let cabalID: String

    var body: some View {
        CabalPotModelHost(cabalID: cabalID) { model in
            ProposePotTotalContent(state: model?.state ?? .loading)
        }
    }
}

struct ProposeCashToSpendRow: View {
    let micros: Int64?
    let failed: Bool

    var body: some View {
        ProposeSummaryRow(title: "Cash to spend", identifier: "propose-amount-cash") {
            if let micros {
                Text(UsdAmountFormatter.format(micros: micros))
                    .moneyFont(.row)
                    .foregroundStyle(MonacoTheme.ink)
                    .multilineTextAlignment(.trailing)
            } else if failed {
                Text("Unavailable")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            } else {
                SkeletonBlock(width: 90, height: 20)
                    .accessibilityLabel("Loading")
            }
        }
    }
}

struct ProposePotTotalContent: View {
    let state: LoadState<CabalPotSummary>

    var body: some View {
        ProposeSummaryRow(title: "Cabal pot", identifier: "propose-amount-pot-total") { value }
    }

    @ViewBuilder private var value: some View {
        switch state {
        case .idle, .loading:
            SkeletonBlock(width: 90, height: 20)
                .accessibilityLabel("Loading")
        case .failed:
            Text("Unavailable")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        case .loaded(let summary):
            Text(summary.potValue)
                .moneyFont(.row)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(.trailing)
        }
    }
}

private struct ProposeSummaryRow<Value: View>: View {
    let title: String
    let identifier: String
    @ViewBuilder let value: Value

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
            Text(title)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
            Spacer(minLength: MonacoTheme.Space.s)
            value
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(identifier)
    }
}
