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

    @Environment(\.cabalModel) private var cabalModel

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, -(SectionStackMetrics.spacing - MonacoTheme.Space.sm))
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            if let cabal = cabalModel?.cabal, cabal.me == nil {
                Color.clear.frame(height: 0)
            } else {
                SkeletonBlock(
                    height: StatMetrics.cellMinHeight + 2 * StatMetrics.cardPaddingVertical,
                    radius: MonacoTheme.Radius.small
                )
                .accessibilityElement()
                .accessibilityLabel("Loading your share")
                .accessibilityIdentifier("cabal-slice-loading")
            }
        case .loaded(let summary):
            if summary.slice != nil {
                StatGrid(items: Self.items(summary, openVotes: Int(cabalModel?.cabal?.openProposalCount ?? 0)))
                    .monacoPrivate()
                    .accessibilityElement(children: .contain)
                    .accessibilityIdentifier("cabal-stat-block")
            } else {
                Color.clear.frame(height: 0)
            }
        case .failed:
            Color.clear.frame(height: 0)
        }
    }

    static func items(_ summary: CabalPotSummary, openVotes: Int) -> [StatItem] {
        CabalHeaderCopy.stats(shareMicros: summary.shareMicros, sliceBps: summary.sliceBps, openVotes: openVotes)
            .map { StatItem(label: $0.label, value: $0.value) }
    }
}
