import MonacoCore
import SwiftUI

struct MonacoChipLabel: ViewModifier {
    static let capsuleHeight: CGFloat = 36
    static let symbolGap: CGFloat = 4

    let isSelected: Bool
    var isPreset = false
    var leadingSymbol: MonacoIcon?
    var fillsWidth = false
    @Environment(\.isEnabled) private var isEnabled
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @ScaledMetric(relativeTo: .subheadline) private var symbolSize: CGFloat = 15

    func body(content: Content) -> some View {
        HStack(spacing: Self.symbolGap) {
            if isSelected, let leadingSymbol {
                Image(systemName: leadingSymbol.symbol)
                    .font(.system(size: symbolSize, weight: .semibold))
                    .accessibilityHidden(true)
            }
            content
        }
        .font(MonacoTheme.Typo.subheadStrong)
        .foregroundStyle(Self.colors(isSelected: isSelected, isEnabled: isEnabled).label)
        .lineLimit(1)
        .minimumScaleFactor(0.8)
        .padding(.horizontal, MonacoTheme.Space.m)
        .frame(maxWidth: fillsWidth ? .infinity : nil, minHeight: Self.capsuleHeight)
        .background(Capsule().fill(Self.colors(isSelected: isSelected, isEnabled: isEnabled).fill))
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .contentShape(Rectangle())
        .animation(reduceMotion ? nil : .spring(response: 0.3, dampingFraction: 0.85), value: isSelected)
    }

    static func colors(isSelected: Bool, isEnabled: Bool) -> (fill: Color, label: Color) {
        if !isEnabled { return (MonacoTheme.surfaceSunken, MonacoTheme.disabledLabel) }
        return isSelected
            ? (MonacoTheme.brandFill, MonacoTheme.onBrand) : (MonacoTheme.surfaceSunken, MonacoTheme.primaryText)
    }
}

extension View {
    func monacoChipLabel(
        isSelected: Bool, isPreset: Bool = false, leadingSymbol: MonacoIcon? = nil, fillsWidth: Bool = false
    ) -> some View {
        modifier(
            MonacoChipLabel(
                isSelected: isSelected, isPreset: isPreset, leadingSymbol: leadingSymbol, fillsWidth: fillsWidth))
    }
}

enum MonacoPlainLabelMetrics {
    static let scrollSpacing: CGFloat = 16
    static let equalWidthFloor = DynamicTypeSize.xxLarge
}

struct MonacoPlainLabelRow<Item: Hashable>: View {
    let items: [Item]
    let selection: Item
    let spacing: CGFloat
    let equalWidth: Bool
    let alwaysFits: Bool
    var scrollInset = MonacoTheme.Space.gutter
    var minLabelWidth: CGFloat?
    let label: (Item) -> String
    let accessibilityName: (Item) -> String
    let identifier: (Item) -> String
    let onSelect: (Item) -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var hitOutset: CGFloat { equalWidth ? 0 : spacing / 2 }

    var body: some View {
        if alwaysFits || dynamicTypeSize >= MonacoPlainLabelMetrics.equalWidthFloor {
            ViewThatFits(in: .horizontal) {
                stack
                scroller
            }
        } else {
            stack
        }
    }

    private var stack: some View {
        HStack(spacing: equalWidth ? 0 : spacing) {
            ForEach(items, id: \.self) { item in plainLabel(item, fillsWidth: equalWidth) }
        }
    }

    private var scroller: some View {
        ScrollViewReader { proxy in
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: MonacoPlainLabelMetrics.scrollSpacing) {
                    ForEach(items, id: \.self) { item in plainLabel(item, fillsWidth: false).id(item) }
                }
                .padding(.horizontal, scrollInset)
            }
            .onAppear { proxy.scrollTo(selection, anchor: .center) }
            .onChange(of: selection) { _, value in
                withAnimation { proxy.scrollTo(value, anchor: .center) }
            }
        }
    }

    private func plainLabel(_ item: Item, fillsWidth: Bool) -> some View {
        let isSelected = item == selection
        return Button {
            guard !isSelected else { return }
            Haptics.selection()
            onSelect(item)
        } label: {
            Text(label(item))
                .font(isSelected ? MonacoTheme.Typo.subheadStrong : MonacoTheme.Typo.subhead)
                .foregroundStyle(isSelected ? MonacoTheme.primaryText : MonacoTheme.secondaryText)
                .lineLimit(1)
                .fixedSize(horizontal: true, vertical: false)
                .frame(
                    minWidth: minLabelWidth,
                    maxWidth: fillsWidth ? .infinity : nil,
                    minHeight: MonacoButtonMetrics.hitTarget
                )
                .padding(.horizontal, hitOutset)
                .contentShape(Rectangle())
                .padding(.horizontal, -hitOutset)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(accessibilityName(item))
        .accessibilityAddTraits(isSelected ? [.isSelected] : [])
        .accessibilityIdentifier(identifier(item))
    }
}

struct MonacoRangeChips<Range: Hashable>: View {
    let ranges: [Range]
    let selection: Range
    let label: (Range) -> String
    let accessibilityName: (Range) -> String
    let identifier: (Range) -> String
    let onSelect: (Range) -> Void

    init(
        ranges: [Range], selection: Range, identifierPrefix: String,
        label: @escaping (Range) -> String, onSelect: @escaping (Range) -> Void
    ) {
        self.ranges = ranges
        self.selection = selection
        self.label = label
        accessibilityName = label
        identifier = { "\(identifierPrefix)-\(label($0))" }
        self.onSelect = onSelect
    }

    var body: some View {
        MonacoPlainLabelRow(
            items: ranges, selection: selection, spacing: 0, equalWidth: true, alwaysFits: false,
            label: label, accessibilityName: accessibilityName, identifier: identifier, onSelect: onSelect)
    }
}

struct MonacoRangeChipsSkeleton: View {
    static let dashWidth: CGFloat = 20
    static let dashHeight: CGFloat = 8

    let ranges: [LeaderboardRange]

    var body: some View {
        HStack(spacing: 0) {
            ForEach(ranges, id: \.self) { _ in
                SkeletonBlock(width: Self.dashWidth, height: Self.dashHeight, radius: Self.dashHeight / 2)
                    .frame(maxWidth: .infinity)
            }
        }
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .accessibilityHidden(true)
    }
}

extension MonacoRangeChips where Range == LeaderboardRange {
    init(
        ranges: [LeaderboardRange], selection: LeaderboardRange, identifierPrefix: String,
        onSelect: @escaping (LeaderboardRange) -> Void
    ) {
        self.ranges = ranges
        self.selection = selection
        label = { $0.label }
        accessibilityName = { $0.accessibilityName }
        identifier = { "\(identifierPrefix)-range-\($0.rawValue)" }
        self.onSelect = onSelect
    }
}

extension LeaderboardRange {
    var accessibilityName: String {
        switch self {
        case .oneHour: "Past hour"
        case .oneDay: "Past day"
        case .oneWeek: "Past week"
        case .oneMonth: "Past month"
        case .threeMonths: "Past 3 months"
        case .sixMonths: "Past 6 months"
        case .all: "All time"
        }
    }
}
