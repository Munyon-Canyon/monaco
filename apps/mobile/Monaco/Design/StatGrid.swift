import SwiftUI

struct StatItem {
    enum Tone: Equatable {
        case plain, gain, loss
    }

    let label: String
    let value: String
    var tone: Tone = .plain
    var info: (() -> Void)? = nil
    var spokenValue: String? = nil
}

extension StatItem: Equatable {
    static func == (lhs: StatItem, rhs: StatItem) -> Bool {
        lhs.label == rhs.label && lhs.value == rhs.value && lhs.tone == rhs.tone
            && lhs.spokenValue == rhs.spokenValue
            && (lhs.info == nil) == (rhs.info == nil)
    }
}

enum StatMetrics {
    static let columnGap: CGFloat = 12
    static let cellMinHeight: CGFloat = 56
    static let pairGap: CGFloat = 20
    static let cardPaddingVertical: CGFloat = 12
    static let cardPaddingHorizontal: CGFloat = 16
    static let infoGlyph: CGFloat = 17
    static let infoTarget: CGFloat = 44
    static let defaultColumns = 4
    static let wideTextColumns = 2

    static func columns(requested: Int, dynamicTypeSize: DynamicTypeSize) -> Int {
        if dynamicTypeSize.isAccessibilitySize { return 1 }
        if dynamicTypeSize >= .xxLarge { return min(requested, wideTextColumns) }
        return requested
    }

    static func color(for tone: StatItem.Tone) -> Color {
        switch tone {
        case .plain: return MonacoTheme.primaryText
        case .gain: return MonacoTheme.profit
        case .loss: return MonacoTheme.loss
        }
    }
}

enum MoneyStripMetrics {
    static let netWorthWeight: CGFloat = 1.4
}

struct StatGrid: View {
    let items: [StatItem]
    var columns: Int = StatMetrics.defaultColumns
    var inCard: Bool = true
    var singleLineValues: Bool = false

    init(
        items: [StatItem], columns: Int = StatMetrics.defaultColumns, inCard: Bool = true,
        singleLineValues: Bool = false
    ) {
        self.items = items
        self.columns = columns
        self.inCard = inCard
        self.singleLineValues = singleLineValues
    }

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    static func rows(_ items: [StatItem], columns: Int) -> [[StatItem]] {
        stride(from: 0, to: items.count, by: columns).map { Array(items[$0..<min($0 + columns, items.count)]) }
    }

    var body: some View {
        let count = max(StatMetrics.columns(requested: columns, dynamicTypeSize: dynamicTypeSize), 1)
        grid(count: count)
            .padding(.vertical, inCard ? StatMetrics.cardPaddingVertical : 0)
            .padding(.horizontal, inCard ? StatMetrics.cardPaddingHorizontal : 0)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background {
                if inCard {
                    RoundedRectangle(cornerRadius: MonacoTheme.Radius.small, style: .continuous)
                        .fill(MonacoTheme.surface)
                }
            }
    }

    private func grid(count: Int) -> some View {
        VStack(spacing: StatMetrics.columnGap) {
            ForEach(Array(Self.rows(items, columns: count).enumerated()), id: \.offset) { _, row in
                HStack(alignment: .lastTextBaseline, spacing: StatMetrics.columnGap) {
                    ForEach(Array(row.enumerated()), id: \.offset) { _, item in
                        cell(item, lineLimit: valueLineLimit(count: count))
                    }
                    if row.count < count {
                        ForEach(0..<(count - row.count), id: \.self) { _ in
                            Color.clear.frame(maxWidth: .infinity, maxHeight: 0)
                        }
                    }
                }
            }
        }
    }

    private func valueLineLimit(count: Int) -> Int? {
        if singleLineValues { return dynamicTypeSize.isAccessibilitySize ? nil : 1 }
        return count > 4 ? 1 : 2
    }

    private func cell(_ item: StatItem, lineLimit: Int?) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 0) {
                Text(item.label)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 1)
                    .minimumScaleFactor(0.75)
                    .fixedSize(horizontal: false, vertical: dynamicTypeSize.isAccessibilitySize)
                    .accessibilityLabel("\(item.label), \(item.spokenValue ?? item.value)")
                if let info = item.info {
                    Button(action: info) {
                        Image(systemName: "info.circle")
                            .font(.system(size: StatMetrics.infoGlyph))
                            .foregroundStyle(MonacoTheme.tint)
                            .frame(width: StatMetrics.infoTarget, height: StatMetrics.infoTarget)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .padding(-(StatMetrics.infoTarget - StatMetrics.infoGlyph) / 2)
                    .padding(.leading, MonacoTheme.Space.xs)
                    .accessibilityLabel("About \(item.label)")
                }
            }
            Text(item.value)
                .moneyFont(.card)
                .foregroundStyle(StatMetrics.color(for: item.tone))
                .lineLimit(lineLimit)
                .minimumScaleFactor(singleLineValues ? 0.6 : 0.8)
                .accessibilityHidden(item.info != nil)
        }
        .frame(maxWidth: .infinity, minHeight: StatMetrics.cellMinHeight, alignment: .bottomLeading)
        .modifier(StatCellAccessibility(item: item))
    }
}

private struct StatCellAccessibility: ViewModifier {
    let item: StatItem

    func body(content: Content) -> some View {
        if item.info == nil {
            content
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("\(item.label), \(item.spokenValue ?? item.value)")
        } else {
            content.accessibilityElement(children: .contain)
        }
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
