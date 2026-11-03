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
            .padding(.top, CabalPageMetrics.statsLead)
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
                statGrid(Self.items(summary, openVotes: openVotes))
            } else if isMember {
                statGrid(Self.placeholderItems(openVotes: openVotes, share: "$0", slice: "0%"))
            } else {
                Color.clear.frame(height: 0)
            }
        case .failed:
            if isMember {
                statGrid(Self.placeholderItems(openVotes: openVotes, share: Self.unknown, slice: Self.unknown))
            } else {
                Color.clear.frame(height: 0)
            }
        }
    }

    static let unknown = "\u{2013}"

    private var isMember: Bool { cabalModel?.cabal?.me != nil }

    private var openVotes: Int { Int(cabalModel?.cabal?.openProposalCount ?? 0) }

    private func statGrid(_ items: [StatItem]) -> some View {
        StatGrid(items: items)
            .monacoPrivate()
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabal-stat-block")
    }

    static func placeholderItems(openVotes: Int, share: String, slice: String) -> [StatItem] {
        var items = [
            StatItem(label: "Your share", value: share),
            StatItem(label: "Of treasury", value: slice),
            StatItem(label: "Open votes", value: "\(openVotes)"),
        ]
        if LaunchFeatures.bots { items.append(StatItem(label: "Bot", value: CabalHeaderCopy.botValue)) }
        return items
    }

    static func items(_ summary: CabalPotSummary, openVotes: Int) -> [StatItem] {
        CabalHeaderCopy.stats(shareMicros: summary.shareMicros, sliceBps: summary.sliceBps, openVotes: openVotes)
            .map { StatItem(label: $0.label, value: $0.value) }
    }
}
