import MonacoRanking
import SwiftUI

struct CallBoardHeader: View {
    let title: String
    let range: String?
    @Binding var selection: CallPeriod

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: 0) {
                    line
                        .multilineTextAlignment(.leading)
                        .fixedSize(horizontal: false, vertical: true)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .accessibilityIdentifier("leaderboard-period-line")
                    ViewThatFits(in: .horizontal) {
                        labels
                        ScrollView(.horizontal, showsIndicators: false) { labels }
                    }
                }
            } else {
                HStack(spacing: MonacoTheme.Space.m) {
                    line
                        .multilineTextAlignment(.leading)
                        .fixedSize(horizontal: false, vertical: true)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .accessibilityIdentifier("leaderboard-period-line")
                    labels
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var labels: some View {
        HStack(spacing: MonacoTheme.Space.m) {
            ForEach(CallPeriod.allCases, id: \.self) { period in
                Button {
                    selection = period
                } label: {
                    Text(CallBoardCopy.periodLabel(period))
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(
                            period == selection ? MonacoTheme.primaryText : MonacoTheme.secondaryText
                        )
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)
                        .frame(
                            minWidth: MonacoButtonMetrics.hitTarget, minHeight: MonacoUnderlineTabsMetrics.minHeight
                        )
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityAddTraits(period == selection ? [.isSelected] : [])
                .accessibilityIdentifier("leaderboard-period-\(period.rawValue)")
            }
        }
    }

    private var line: Text {
        let head = Text(title).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
        guard let range else { return head }
        return head + Text(" \(range)").font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
    }
}
