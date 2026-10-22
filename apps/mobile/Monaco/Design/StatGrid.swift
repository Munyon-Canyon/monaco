import SwiftUI

struct StatItem: Equatable {
    let label: String
    let value: String
}

enum StatMetrics {
    static let columnGap: CGFloat = 24
    static let cellMinHeight: CGFloat = 72
    static let pairGap: CGFloat = 20
}

enum MoneyStripMetrics {
    static let netWorthWeight: CGFloat = 1.4
}

struct StatGrid: View {
    let items: [StatItem]

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    static func rows(_ items: [StatItem], columns: Int) -> [[StatItem]] {
        stride(from: 0, to: items.count, by: columns).map { Array(items[$0..<min($0 + columns, items.count)]) }
    }

    var body: some View {
        let columns = dynamicTypeSize.isAccessibilitySize ? 1 : 2
        VStack(spacing: 0) {
            ForEach(Array(Self.rows(items, columns: columns).enumerated()), id: \.offset) { _, row in
                HStack(alignment: .top, spacing: StatMetrics.columnGap) {
                    ForEach(Array(row.enumerated()), id: \.offset) { _, item in
                        cell(item)
                    }
                    if row.count < columns {
                        Color.clear.frame(maxWidth: .infinity, maxHeight: 0)
                    }
                }
            }
        }
    }

    private func cell(_ item: StatItem) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(item.label)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            Text(item.value)
                .moneyFont(.row)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
        }
        .padding(.vertical, MonacoTheme.Space.m)
        .frame(
            maxWidth: .infinity, minHeight: StatMetrics.cellMinHeight, maxHeight: .infinity, alignment: .topLeading
        )
        .overlay(alignment: .bottom) { MonacoRule(color: MonacoTheme.border) }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(item.label), \(item.value)")
    }
}

struct StatPair: View {
    let items: [StatItem]

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var layout: AnyLayout {
        dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: StatMetrics.pairGap))
            : AnyLayout(HStackLayout(alignment: .top, spacing: StatMetrics.pairGap))
    }

    var body: some View {
        VStack(spacing: 0) {
            MonacoRule(color: MonacoTheme.border)
            layout {
                ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                    VStack(alignment: .leading, spacing: 0) {
                        Text(item.label)
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.secondaryText)
                        Text(item.value)
                            .moneyFont(.card)
                            .foregroundStyle(MonacoTheme.primaryText)
                            .lineLimit(1)
                            .minimumScaleFactor(MoneyStyle.card.minimumScaleFactor)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel("\(item.label), \(item.value)")
                }
            }
            .padding(.vertical, MonacoTheme.Space.m)
            MonacoRule(color: MonacoTheme.border)
        }
    }
}

struct MoneyStrip: View {
    let netWorth: String
    let cash: String
    let depositTitle: String
    let onDeposit: () -> Void

    @Environment(\.displayScale) private var displayScale

    var body: some View {
        VStack(spacing: 0) {
            ViewThatFits(in: .horizontal) {
                MoneyStripRow(spacing: MonacoTheme.Space.s, weights: [MoneyStripMetrics.netWorthWeight, 1]) {
                    metric("Net worth", netWorth)
                        .padding(.trailing, MonacoTheme.Space.s)
                        .overlay(alignment: .trailing) {
                            Rectangle().fill(MonacoTheme.border).frame(width: 1 / displayScale)
                        }
                    metric("Cash", cash)
                    deposit(fillsWidth: false)
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    metric("Net worth", netWorth)
                    metric("Cash", cash)
                    deposit(fillsWidth: true)
                }
            }
            .padding(.top, MonacoTheme.Space.m)
            .overlay(alignment: .top) { MonacoRule(color: MonacoTheme.border) }
        }
    }

    private func metric(_ label: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(label)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            Text(value)
                .moneyFont(.row)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(label), \(value)")
    }

    private func deposit(fillsWidth: Bool) -> some View {
        Button(action: onDeposit) {
            Text(depositTitle).frame(maxWidth: fillsWidth ? .infinity : nil)
        }
        .buttonStyle(.monacoCompactProminent)
    }
}

private struct MoneyStripRow: Layout {
    let spacing: CGFloat
    let weights: [CGFloat]

    private func widths(_ subviews: Subviews, in total: CGFloat) -> [CGFloat] {
        let button = subviews[2].sizeThatFits(.unspecified).width
        let room = max(total - button - spacing * 2, 0)
        let shares = weights.reduce(0, +)
        return weights.map { room * $0 / shares } + [button]
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        guard subviews.count == 3 else { return .zero }
        let ideals = subviews.map { $0.sizeThatFits(.unspecified).width }
        let shares = weights.reduce(0, +)
        let metrics = zip(weights, ideals).map { $1 * shares / $0 }.max() ?? 0
        let width = proposal.width ?? metrics + ideals[2] + spacing * 2
        let heights = zip(subviews, widths(subviews, in: width)).map {
            $0.sizeThatFits(ProposedViewSize(width: $1, height: nil)).height
        }
        return CGSize(width: width, height: heights.max() ?? 0)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        guard subviews.count == 3 else { return }
        var x = bounds.minX
        for (subview, width) in zip(subviews, widths(subviews, in: bounds.width)) {
            subview.place(
                at: CGPoint(x: x, y: bounds.midY), anchor: .leading,
                proposal: ProposedViewSize(width: width, height: nil))
            x += width + spacing
        }
    }
}
