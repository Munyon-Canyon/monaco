import MonacoCore
import SwiftUI

struct AssetMarketStats: View {
    let stats: AssetStats
    let showInfo: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: 0) {
                Text(AssetStatsInfoCopy.title)
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .accessibilityAddTraits(.isHeader)
                Button(action: showInfo) {
                    Image(systemName: "info.circle")
                        .font(MonacoTheme.Typo.section)
                        .foregroundStyle(MonacoTheme.tint)
                        .frame(width: StatMetrics.infoTarget, height: StatMetrics.infoTarget)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("About market stats")
                .accessibilityIdentifier("asset-stats-info-button")
                Spacer(minLength: 0)
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
