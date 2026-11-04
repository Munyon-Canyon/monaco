import MonacoCore
import SwiftUI

struct AssetMarketStats: View {
    let stats: AssetStats
    let showInfo: () -> Void
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var heading: some View {
        Text(AssetStatsInfoCopy.title)
            .font(MonacoTheme.Typo.section)
            .foregroundStyle(MonacoTheme.primaryText)
            .accessibilityAddTraits(.isHeader)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if dynamicTypeSize.isAccessibilitySize {
                HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.xs) {
                    heading.fixedSize(horizontal: false, vertical: true)
                    AssetMarketStatsInfoButton(action: showInfo).fixedSize()
                    Spacer(minLength: 0)
                }
            } else {
                HStack(spacing: 0) {
                    heading
                    AssetMarketStatsInfoButton(action: showInfo)
                    Spacer(minLength: 0)
                }
            }
            StatGrid(items: Self.items(stats.topRow), columns: 3)
            StatGrid(items: Self.items(stats.bottomRow), columns: 3)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-stats")
    }

    static func items(_ cells: [AssetStatCell]) -> [StatItem] {
        cells.map { StatItem(label: $0.label, value: $0.value, spokenValue: $0.spokenValue) }
    }
}

private struct AssetMarketStatsInfoButton: View {
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: "info.circle")
                .font(MonacoTheme.Typo.section)
                .dynamicTypeSize(...DynamicTypeSize.xxxLarge)
                .foregroundStyle(MonacoTheme.tint)
                .frame(width: StatMetrics.infoTarget, height: StatMetrics.infoTarget)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("About market stats")
        .accessibilityIdentifier("asset-stats-info-button")
    }
}

struct AssetAbout: View {
    let detail: AssetDetailPresentation

    static func isShown(kind: AssetKind, hasProfile: Bool = LaunchFeatures.companyProfile) -> Bool {
        kind == .preIpo || hasProfile
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(detail.name)
            if detail.kind == .preIpo {
                Text(PreIpoCopy.disclosure)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.bodyText)
                    .lineSpacing(MonacoTheme.Space.xs)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-about")
    }
}
