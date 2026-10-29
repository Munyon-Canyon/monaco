import MonacoCore
import SwiftUI

struct PreIpoRoundStats: View {
    let top: [AssetStatCell]
    let bottom: [AssetStatCell]
    let showInfo: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: 0) {
                Text(PreIpoCopy.roundStatsTitle)
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
                .accessibilityLabel(PreIpoCopy.roundStatsInfoLabel)
                .accessibilityIdentifier("asset-pre-ipo-stats-info-button")
                Spacer(minLength: 0)
            }
            StatGrid(items: AssetMarketStats.items(top), columns: 3)
            StatGrid(items: AssetMarketStats.items(bottom), columns: 2)
            Text(PreIpoCopy.roundStatsFootnote)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.horizontal, StatMetrics.cardPaddingHorizontal)
                .accessibilityIdentifier("asset-pre-ipo-footnote")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-pre-ipo-stats")
    }
}

struct PreIpoInfoSheet: View {
    let title: String
    let text: String
    let identifier: String

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.secondaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.m)
            .frame(maxWidth: .infinity, alignment: .leading)
            .monacoFittedSheet(title: title, titleIdentifier: "\(identifier)-title", showsClose: true)
            .accessibilityIdentifier(identifier)
    }
}
