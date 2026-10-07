import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalSliceSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalPotModelHost(cabalID: context.cabalID) { model in
            CabalSliceBand(model: model)
        }
    }
}

struct CabalSliceBand: View {
    let model: CabalPotModel?

    var body: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            band {
                SkeletonBlock(width: 140, height: 28)
                SkeletonBlock(width: 100, height: 14)
            }
        case .loaded(let summary):
            if let slice = summary.slice {
                band { figures(slice) }
            } else {
                Color.clear.frame(height: 0)
            }
        case .failed:
            Color.clear.frame(height: 0)
        }
    }

    private func band(@ViewBuilder _ content: () -> some View) -> some View {
        CabalInkBand {
            Rectangle()
                .fill(MonacoTheme.onHeroHairline)
                .frame(height: 1)
                .padding(.bottom, MonacoTheme.Space.s)
                .accessibilityHidden(true)
            content()
        }
    }

    private func figures(_ slice: CabalPotSummary.Slice) -> some View {
        ViewThatFits(in: .horizontal) {
            HStack(alignment: .lastTextBaseline) {
                value(slice)
                Spacer(minLength: MonacoTheme.Space.m)
                share(slice, alignment: .trailing)
            }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                value(slice)
                share(slice, alignment: .leading)
            }
        }
    }

    private func value(_ slice: CabalPotSummary.Slice) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text("Your slice")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.onHero)
                .accessibilityAddTraits(.isHeader)
            Text(slice.value)
                .moneyFont(.large)
                .foregroundStyle(MonacoTheme.onHero)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.large.minimumScaleFactor)
                .accessibilityIdentifier("cabal-slice-value")
        }
    }

    @ViewBuilder
    private func share(_ slice: CabalPotSummary.Slice, alignment: HorizontalAlignment) -> some View {
        switch slice {
        case .none:
            Text("Fund to get a slice")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.onHeroMuted)
                .accessibilityIdentifier("cabal-slice-none")
        case .stake(_, let ofPot, let gain):
            VStack(alignment: alignment, spacing: 2) {
                Text(ofPot)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.onHeroMuted)
                    .accessibilityIdentifier("cabal-slice-share")
                PnLText(dollarPnl: gain, style: .caption, onInk: true)
                    .accessibilityIdentifier("cabal-slice-gain")
            }
        }
    }
}

extension CabalPotSummary.Slice {
    var value: String {
        switch self {
        case .none: "$0.00"
        case .stake(let value, _, _): value
        }
    }
}
