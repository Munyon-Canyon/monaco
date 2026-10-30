import MonacoCore
import SwiftUI

struct ProposalStatRow: View {
    let stats: ProposalStats
    var showsInfo = true
    var onInfo: () -> Void = {}

    var body: some View {
        StatGrid(items: Self.items(stats, showsInfo: showsInfo, onInfo: onInfo), singleLineValues: true)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("proposal-stat-row")
    }

    static func items(_ stats: ProposalStats, showsInfo: Bool, onInfo: @escaping () -> Void) -> [StatItem] {
        stats.cells.map { cell in
            StatItem(
                label: cell.label, value: cell.value, tone: cell.tone.statTone,
                info: showsInfo && cell.hasInfo ? onInfo : nil)
        }
    }
}

extension ProposalStats.Tone {
    fileprivate var statTone: StatItem.Tone {
        switch self {
        case .plain: .plain
        case .gain: .gain
        case .loss: .loss
        }
    }
}
