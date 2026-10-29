import MonacoRanking
import SwiftUI

struct CallBoardHeader: View {
    let title: String
    let range: String?
    @Binding var selection: CallPeriod

    var body: some View {
        HStack(spacing: MonacoTheme.Space.m) {
            line
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("leaderboard-period-line")
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
                            .frame(minHeight: MonacoUnderlineTabsMetrics.minHeight)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(period == selection ? [.isSelected] : [])
                    .accessibilityIdentifier("leaderboard-period-\(period.rawValue)")
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var line: Text {
        let head = Text(title).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
        guard let range else { return head }
        return head + Text(" \(range)").font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
    }
}
