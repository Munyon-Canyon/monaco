import Charts
import MonacoCore
import SwiftUI

struct CabalLinesChart: View {
    struct Line: Identifiable {
        let id: String
        let name: String
        let points: [CurvePoint]
    }

    let lines: [Line]
    let range: LeaderboardRange
    var height: CGFloat = 160
    let identifier: String

    private static let unitsPerDollar = 1_000_000.0

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Chart {
                ForEach(lines) { line in
                    ForEach(Array(line.points.enumerated()), id: \.offset) { _, point in
                        LineMark(
                            x: .value("Time", point.at),
                            y: .value("Return", Double(point.value) / Self.unitsPerDollar),
                            series: .value("Cabal", line.id)
                        )
                        .foregroundStyle(MonacoTheme.CabalTint.forGroupId(line.id).fill)
                        .lineStyle(StrokeStyle(lineWidth: 2, lineCap: .round, lineJoin: .round))
                        .interpolationMethod(.monotone)
                    }
                }
            }
            .chartLegend(.hidden)
            .chartXAxis(.hidden)
            .chartYAxis(.hidden)
            .chartYScale(domain: .automatic(includesZero: false))
            .frame(height: height)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Return of each cabal, \(range.accessibilityName)")
            .accessibilityValue(lines.map(\.name).joined(separator: ", "))
            .accessibilityIdentifier(identifier)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: MonacoTheme.Space.m) {
                    ForEach(lines) { line in
                        HStack(spacing: 6) {
                            Circle()
                                .fill(MonacoTheme.CabalTint.forGroupId(line.id).fill)
                                .frame(width: 8, height: 8)
                            Text(line.name)
                                .font(MonacoTheme.Typo.caption)
                                .foregroundStyle(MonacoTheme.muted)
                                .lineLimit(1)
                        }
                    }
                }
            }
            .accessibilityHidden(true)
        }
    }
}
