import SwiftUI

struct QuietChart: View {
    let message: String

    var body: some View {
        Text(message)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .multilineTextAlignment(.center)
            .padding(.horizontal, MonacoTheme.Space.m)
            .frame(maxWidth: .infinity, minHeight: ChartMetrics.quietHeight)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.field, style: .continuous)
                    .fill(MonacoTheme.surface)
            )
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(message)
    }
}
