import MonacoCore
import SwiftUI

enum FeedResultMetrics {
    static let seal: CGFloat = 22
    static let sparklineWidth: CGFloat = 96
    static let sparklineHeight: CGFloat = 28
}

struct FeedResultAttachment: View {
    let sealName: String
    let sparkline: [FeedPostFacts.ChartPoint]
    let isLoss: Bool

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            Image(systemName: "checkmark.seal.fill")
                .resizable()
                .scaledToFit()
                .frame(width: FeedResultMetrics.seal, height: FeedResultMetrics.seal)
                .foregroundStyle(MonacoTheme.tint)
                .accessibilityHidden(true)
            Text(ResultCopy.calledIt(sealName))
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
                .monacoPrivate()
            Spacer(minLength: MonacoTheme.Space.s)
            if let series = SparklineSeries(usdcMicros: sparkline.map(\.priceMicros)) {
                Sparkline(
                    series: series, tone: isLoss ? .loss : .profit, width: FeedResultMetrics.sparklineWidth,
                    height: FeedResultMetrics.sparklineHeight)
            }
        }
        .padding(.vertical, MonacoTheme.Space.s)
    }
}
