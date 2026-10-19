import SwiftUI

enum AllocationBarMetrics {
    static let barHeight: CGFloat = 7
    static let gap: CGFloat = 2
    static let dotSize: CGFloat = 8
}

struct AllocationBar: View {
    struct Slice: Equatable {
        let label: String
        let fraction: Double
        var isCash = false
    }

    let slices: [Slice]
    var showsLegend = true

    static func tintIndexes(for slices: [Slice]) -> [Int] {
        let last = MonacoTheme.allocationTints.count - 1
        var holdings = 0
        return slices.map { slice in
            if slice.isCash { return last }
            defer { holdings += 1 }
            return min(holdings, last - 1)
        }
    }

    private var tints: [Color] {
        Self.tintIndexes(for: slices).map { MonacoTheme.allocationTints[$0] }
    }

    private var spokenSummary: String {
        let parts = slices.map { "\($0.label) \(Self.percentNumber($0.fraction)) percent" }
        return "Allocation: " + parts.joined(separator: ", ") + "."
    }

    private static func percentNumber(_ fraction: Double) -> String {
        (fraction * 100).formatted(.number.precision(.fractionLength(0...1)))
    }

    private static func legendText(_ slice: Slice) -> String {
        "\(slice.label) \(slice.fraction.formatted(.percent.precision(.fractionLength(0...1))))"
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            SegmentedBar(
                segments: zip(slices, tints).enumerated().map {
                    SegmentedBar.Segment(id: $0.offset, weight: $0.element.0.fraction, color: $0.element.1)
                },
                height: AllocationBarMetrics.barHeight, gap: AllocationBarMetrics.gap
            )
            .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.badge, style: .continuous))
            if showsLegend {
                legend
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spokenSummary)
    }

    private var legend: some View {
        AllocationLegendLayout(spacing: MonacoTheme.Space.sm, lineSpacing: MonacoTheme.Space.xs) {
            ForEach(Array(zip(slices, tints).enumerated()), id: \.offset) { _, pair in
                HStack(spacing: MonacoTheme.Space.xs) {
                    Circle()
                        .fill(pair.1)
                        .frame(width: AllocationBarMetrics.dotSize, height: AllocationBarMetrics.dotSize)
                    Text(Self.legendText(pair.0))
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
            }
        }
    }
}

private struct AllocationLegendLayout: Layout {
    let spacing: CGFloat
    let lineSpacing: CGFloat

    private func fit(_ subview: LayoutSubview, in width: CGFloat) -> CGSize {
        subview.sizeThatFits(ProposedViewSize(width: width.isFinite ? width : nil, height: nil))
    }

    private func rows(in width: CGFloat, _ subviews: Subviews) -> [[Int]] {
        var rows: [[Int]] = [[]]
        var used: CGFloat = 0
        for (index, subview) in subviews.enumerated() {
            let size = fit(subview, in: width)
            let needed = rows[rows.count - 1].isEmpty ? size.width : used + spacing + size.width
            if needed > width, !rows[rows.count - 1].isEmpty {
                rows.append([index])
                used = size.width
            } else {
                rows[rows.count - 1].append(index)
                used = needed
            }
        }
        return rows
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let width = proposal.width ?? .infinity
        let laidOut = rows(in: width, subviews)
        var height: CGFloat = 0
        var widest: CGFloat = 0
        for row in laidOut {
            let sizes = row.map { fit(subviews[$0], in: width) }
            height += (sizes.map(\.height).max() ?? 0) + (height == 0 ? 0 : lineSpacing)
            widest = max(widest, sizes.map(\.width).reduce(0, +) + spacing * CGFloat(max(row.count - 1, 0)))
        }
        return CGSize(width: widest, height: height)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var y = bounds.minY
        for row in rows(in: bounds.width, subviews) {
            var x = bounds.minX
            var rowHeight: CGFloat = 0
            for index in row {
                let size = fit(subviews[index], in: bounds.width)
                subviews[index].place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(size))
                x += size.width + spacing
                rowHeight = max(rowHeight, size.height)
            }
            y += rowHeight + lineSpacing
        }
    }
}
