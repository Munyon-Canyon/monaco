import MonacoCore
import SwiftUI

struct MonacoChipLabel: ViewModifier {
    let isSelected: Bool
    var isPreset = false
    @Environment(\.isEnabled) private var isEnabled
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        content
            .font(MonacoTheme.Typo.subheadStrong)
            .foregroundStyle(foreground)
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .padding(.horizontal, isPreset ? MonacoTheme.Space.m : MonacoTheme.Space.sm)
            .frame(minHeight: isPreset ? 36 : 32)
            .background(Capsule().fill(fill))
            .frame(minHeight: 44)
            .contentShape(Rectangle())
            .animation(reduceMotion ? nil : .spring(response: 0.3, dampingFraction: 0.85), value: isSelected)
    }

    private var foreground: Color {
        if !isEnabled { return MonacoTheme.disabledLabel }
        if isSelected { return MonacoTheme.onBrand }
        return isPreset ? MonacoTheme.ink : MonacoTheme.secondaryText
    }

    private var fill: Color {
        if isSelected { return MonacoTheme.brandFill }
        return isPreset ? MonacoTheme.surfaceSunken : .clear
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
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: MonacoTheme.Space.s) {
                ForEach(ranges, id: \.self) { range in
                    let isSelected = range == selection
                    Button {
                        guard !isSelected else { return }
                        Haptics.selection()
                        onSelect(range)
                    } label: {
                        Text(label(range)).monacoChipLabel(isSelected: isSelected)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(accessibilityName(range))
                    .accessibilityAddTraits(isSelected ? [.isSelected] : [])
                    .accessibilityIdentifier(identifier(range))
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        }
        .padding(.horizontal, -MonacoTheme.Space.gutter)
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
