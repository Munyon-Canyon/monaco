import Foundation

/// Deterministic text snapshots of the group screen for fixture regression tests.
/// Mirrors visible copy from SwiftUI product views without UIKit/SwiftUI dependencies.
public enum ScreenSnapshotRenderer {
    public static func groupScreen(from view: GroupViewDTO) -> String {
        var lines: [String] = ["# \(view.name)", "", "## Pot"]
        if view.pot.isEmpty {
            lines.append("No holdings yet. Fund this cabal to get started.")
        } else {
            lines.append("Total | $\(view.resolvedPotTotalUsd)")
            for row in view.pot {
                var headline = "\(row.symbol) | $\(row.valueUsd)"
                if row.afterHours == true {
                    headline += " [After hours]"
                }
                lines.append(headline)
                lines.append("  \(row.dollarPnl)")
                lines.append("  \(row.units) units @ $\(row.markUsd)")
            }
        }

        lines.append("")
        lines.append("## You")
        lines.append("Your slice | $\(view.you.equityUsd)")
        lines.append("Slice % | \(formatSlicePercent(view.you.slicePercent))")
        lines.append("P&L | \(view.you.dollarPnl)")
        lines.append("Return | \(PercentReturnFormatter.format(view.you.percentReturn))")

        return lines.joined(separator: "\n")
    }

    private static func formatSlicePercent(_ raw: String) -> String {
        SlicePercentFormatter.format(raw)
    }
}
