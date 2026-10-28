import SwiftUI

struct QuietChart: View {
    let message: String

    var body: some View {
        ZStack {
            DashedBaseline()
                .stroke(MonacoTheme.hairline, style: StrokeStyle(lineWidth: 1, dash: [3, 4]))
                .frame(height: 1)
                .frame(maxHeight: .infinity, alignment: .bottom)
            Text(message)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .padding(.horizontal, MonacoTheme.Space.m)
        }
        .frame(maxWidth: .infinity, minHeight: ChartMetrics.quietHeight)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(message)
    }
}

private nonisolated struct DashedBaseline: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        path.move(to: CGPoint(x: rect.minX, y: rect.midY))
        path.addLine(to: CGPoint(x: rect.maxX, y: rect.midY))
        return path
    }
}
