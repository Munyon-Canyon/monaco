import MonacoCore
import SwiftUI

struct MonacoChipLabel: ViewModifier {
    static let capsuleHeight: CGFloat = 36
    static let borderWidth: CGFloat = 1

    let isSelected: Bool
    var isPreset = false
    @Environment(\.isEnabled) private var isEnabled
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        content
            .font(isSelected ? MonacoTheme.Typo.captionStrong : MonacoTheme.Typo.caption)
            .foregroundStyle(foreground)
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .padding(.horizontal, MonacoTheme.Space.m)
            .frame(minHeight: Self.capsuleHeight)
            .background {
                if isSelected {
                    Capsule().fill(MonacoTheme.brandSoft)
                } else {
                    Capsule().strokeBorder(MonacoTheme.border, lineWidth: Self.borderWidth)
                }
            }
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(Rectangle())
            .animation(reduceMotion ? nil : .spring(response: 0.3, dampingFraction: 0.85), value: isSelected)
    }

    private var foreground: Color {
        if !isEnabled { return MonacoTheme.disabledLabel }
        return isSelected ? MonacoTheme.brand : MonacoTheme.secondaryText
    }
}

extension View {
    func monacoChipLabel(isSelected: Bool, isPreset: Bool = false) -> some View {
        modifier(MonacoChipLabel(isSelected: isSelected, isPreset: isPreset))
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
        MonacoSegmented(
            ranges,
            selection: Binding(get: { selection }, set: { onSelect($0) }),
            layout: .periods,
            accessibilityName: accessibilityName,
            identifier: identifier,
            label: label
        )
    }
}

struct MonacoRangeChipsSkeleton: View {
    let ranges: [LeaderboardRange]

    var body: some View {
        SkeletonBlock(
            height: MonacoSegmentedMetrics.periodsMinHeight, radius: MonacoSegmentedMetrics.periodsMinHeight / 2
        )
        .frame(maxWidth: .infinity)
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
        case .all: "All time"
        }
    }
}
