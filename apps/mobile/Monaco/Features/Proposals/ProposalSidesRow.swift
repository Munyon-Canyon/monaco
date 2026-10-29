import MonacoCore
import SwiftUI

struct ProposalSidesRow: View {
    let sides: ProposalSides

    private static let avatarSize: CGFloat = 24
    private static let stackLimit = 5

    private struct Line: Identifiable {
        let side: ProposalSides.Side
        let glyph: String
        let color: Color
        var id: String { glyph }
    }

    private let lines = [
        Line(side: .yes, glyph: "✓", color: MonacoTheme.profit),
        Line(side: .no, glyph: "✕", color: MonacoTheme.loss),
        Line(side: .notVoted, glyph: "–", color: MonacoTheme.secondaryText),
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            ForEach(lines) { line in
                let entries = sides.entries(for: line.side)
                if !entries.isEmpty { row(line, entries) }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("proposal-sides")
    }

    private func row(_ line: Line, _ entries: [ProposalSides.Entry]) -> some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text(line.glyph)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(line.color)
                .frame(width: Self.avatarSize)
                .accessibilityHidden(true)
            HStack(spacing: -MonacoTheme.Space.xs) {
                ForEach(entries.prefix(Self.stackLimit)) { entry in
                    MonacoAvatar(
                        photoURL: entry.photoURL?.absoluteString, displayName: entry.name, size: Self.avatarSize,
                        seed: entry.id)
                }
            }
            .opacity(line.side == .notVoted ? 0.5 : 1)
            .accessibilityHidden(true)
            Text(sides.line(for: line.side))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(line.side == .notVoted ? MonacoTheme.secondaryText : MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(sides.spokenLine(for: line.side))
    }
}
