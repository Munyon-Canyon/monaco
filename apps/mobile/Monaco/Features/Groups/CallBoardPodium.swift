import MonacoRanking
import SwiftUI

enum CallBoardPodiumMetrics {
    static let raise: CGFloat = 8
    static let avatar: CGFloat = 40
    static let crownRise: CGFloat = 14
}

struct CallBoardPodium: View {
    let rows: [CallBoardRowValue]

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var ordered: [CallBoardRowValue] {
        guard rows.count == 3 else { return rows }
        return [rows[1], rows[0], rows[2]]
    }

    var body: some View {
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(spacing: MonacoTheme.Space.sm) { places }
            } else {
                HStack(alignment: .top, spacing: MonacoTheme.Space.s) { places }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
    }

    @ViewBuilder private var places: some View {
        ForEach(ordered) { row in
            NavigationLink(value: CallBoardRoutes.route(for: row)) {
                place(row)
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(row.spoken)
            .accessibilityAddTraits(.isButton)
            .accessibilityIdentifier("leaderboard-podium-\(row.rank ?? 0)")
        }
    }

    private func place(_ row: CallBoardRowValue) -> some View {
        let first = row.rank == 1
        return VStack(spacing: MonacoTheme.Space.xs) {
            CallBoardAvatar(row: row, size: CallBoardPodiumMetrics.avatar)
                .overlay(alignment: .top) {
                    if first {
                        Image(systemName: "crown.fill")
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.crown)
                            .offset(y: -CallBoardPodiumMetrics.crownRise)
                            .accessibilityHidden(true)
                    }
                }
            Text("\(row.rate) \(row.movementGlyph)")
                .font(MonacoTheme.Typo.headline).monospacedDigit().foregroundStyle(MonacoTheme.primaryText)
            Text(row.record).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
        }
        .frame(maxWidth: .infinity)
        .offset(y: first && !dynamicTypeSize.isAccessibilitySize ? -CallBoardPodiumMetrics.raise : 0)
    }
}
