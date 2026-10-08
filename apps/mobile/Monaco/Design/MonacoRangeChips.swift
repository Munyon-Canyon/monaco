import MonacoCore
import SwiftUI

struct MonacoChipLabel: ViewModifier {
    let isSelected: Bool
    var onInk = false
    @Environment(\.isEnabled) private var isEnabled

    func body(content: Content) -> some View {
        content
            .font(MonacoTheme.Typo.calloutStrong)
            .foregroundStyle(foreground)
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .padding(.horizontal, MonacoTheme.Space.m)
            .frame(minHeight: 36)
            .background(Capsule().fill(fill))
            .frame(minHeight: 44)
            .contentShape(Rectangle())
    }

    private var foreground: Color {
        if !isEnabled { return MonacoTheme.disabledLabel }
        switch (onInk, isSelected) {
        case (true, true): return MonacoTheme.heroInk
        case (true, false): return MonacoTheme.onHeroMuted
        case (false, true): return MonacoTheme.onBrand
        case (false, false): return MonacoTheme.ink
        }
    }

    private var fill: Color {
        switch (onInk, isSelected) {
        case (true, true): MonacoTheme.onHero
        case (true, false): MonacoTheme.onHeroHairline
        case (false, true): MonacoTheme.brandFill
        case (false, false): MonacoTheme.surfaceSunken
        }
    }
}

extension View {
    func monacoChipLabel(isSelected: Bool, onInk: Bool = false) -> some View {
        modifier(MonacoChipLabel(isSelected: isSelected, onInk: onInk))
    }
}

struct MonacoRangeChips<Range: Hashable>: View {
    let ranges: [Range]
    let selection: Range
    var onInk = false
    let label: (Range) -> String
    let accessibilityName: (Range) -> String
    let identifier: (Range) -> String
    let onSelect: (Range) -> Void

    init(
        ranges: [Range], selection: Range, onInk: Bool = false, identifierPrefix: String,
        label: @escaping (Range) -> String, onSelect: @escaping (Range) -> Void
    ) {
        self.ranges = ranges
        self.selection = selection
        self.onInk = onInk
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
                        onSelect(range)
                    } label: {
                        Text(label(range)).monacoChipLabel(isSelected: isSelected, onInk: onInk)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(accessibilityName(range))
                    .accessibilityAddTraits(isSelected ? [.isSelected] : [])
                    .accessibilityIdentifier(identifier(range))
                }
            }
        }
    }
}

extension MonacoRangeChips where Range == LeaderboardRange {
    init(
        ranges: [LeaderboardRange], selection: LeaderboardRange, onInk: Bool = false, identifierPrefix: String,
        onSelect: @escaping (LeaderboardRange) -> Void
    ) {
        self.ranges = ranges
        self.selection = selection
        self.onInk = onInk
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
