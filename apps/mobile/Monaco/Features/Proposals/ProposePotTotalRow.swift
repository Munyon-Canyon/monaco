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

struct ProposePotTotalContent: View {
    let state: LoadState<CabalPotSummary>

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
            Text("Cabal pot")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
            Spacer(minLength: MonacoTheme.Space.s)
            value
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .background(MonacoTheme.surface, in: cardShape)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("propose-amount-pot-total")
    }

    private var cardShape: RoundedRectangle {
        RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
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
