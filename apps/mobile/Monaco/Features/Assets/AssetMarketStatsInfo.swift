import MonacoCore
import SwiftUI

struct AssetMarketStatsInfo: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            ForEach(AssetStatsInfoCopy.lines, id: \.label) { line in
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(line.label)
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(line.value)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .accessibilityElement(children: .combine)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoFittedSheet(title: AssetStatsInfoCopy.title, titleIdentifier: "asset-stats-info-title", showsClose: true)
        .accessibilityIdentifier("asset-stats-info")
    }
}
