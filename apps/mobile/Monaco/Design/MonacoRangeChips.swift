import MonacoCore
import SwiftUI

struct MonacoRangeChips: View {
    let ranges: [LeaderboardRange]
    let selection: LeaderboardRange
    var onInk = false
    let identifierPrefix: String
    let onSelect: (LeaderboardRange) -> Void

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: MonacoTheme.Space.s) {
                ForEach(ranges, id: \.self) { range in
                    let isSelected = range == selection
                    Button {
                        onSelect(range)
                    } label: {
                        chip(range.label, isSelected: isSelected)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(range.accessibilityName)
                    .accessibilityAddTraits(isSelected ? [.isSelected] : [])
                    .accessibilityIdentifier("\(identifierPrefix)-range-\(range.rawValue)")
                }
            }
        }
    }

    private func chip(_ title: String, isSelected: Bool) -> some View {
        Text(title)
            .font(MonacoTheme.Typo.dataCaption)
            .foregroundStyle(foreground(isSelected))
            .lineLimit(1)
            .padding(.horizontal, 14)
            .frame(minWidth: 48, minHeight: 34)
            .background(Capsule().fill(fill(isSelected)))
            .padding(.vertical, 5)
            .contentShape(Capsule())
    }

    private func foreground(_ isSelected: Bool) -> Color {
        switch (onInk, isSelected) {
        case (true, true): MonacoTheme.heroInk
        case (true, false): MonacoTheme.onHeroMuted
        case (false, true): MonacoTheme.primaryButtonLabel
        case (false, false): MonacoTheme.muted
        }
    }

    private func fill(_ isSelected: Bool) -> Color {
        switch (onInk, isSelected) {
        case (true, true): MonacoTheme.onHero
        case (true, false): MonacoTheme.onHeroHairline
        case (false, true): MonacoTheme.primaryButtonFill
        case (false, false): MonacoTheme.surfaceSunken
        }
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
