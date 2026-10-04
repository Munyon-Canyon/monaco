import MonacoCore
import SwiftUI

/// The one chip a closed proposal shows: Bought / Sold / Didn't pass / Expired / Failed.
/// Open proposals show none.
struct ProposalStatusChip: View {
    private let label: String
    private let status: ProposalStatus
    private let isSell: Bool
    private let swapFailed: Bool

    init(status: ProposalStatus, isSell: Bool, swapFailed: Bool = false) {
        self.status = status
        self.isSell = isSell
        self.swapFailed = swapFailed
        label = ProposalChip.label(status: status, isSell: isSell, swapFailed: swapFailed) ?? ""
    }

    init(label: String, stage: ProposalExecutionStage?, status: String, kind: String = "buy") {
        self.label = label
        self.status = ProposalStatus(rawValue: status) ?? .open
        isSell = kind == "sell"
        swapFailed = stage == .failed
    }

    var body: some View {
        Text(label)
            .font(MonacoTheme.Typo.micro)
            .foregroundStyle(tint)
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .background(Capsule().fill(fill))
            .lineLimit(1)
            .fixedSize()
            .accessibilityIdentifier("proposal-status-\(isSell ? "sell" : "buy")-\(status.rawValue)")
    }

    private var isPositive: Bool {
        status == .passed && !swapFailed
    }

    private var tint: Color {
        if swapFailed { return MonacoTheme.loss }
        return isPositive ? MonacoTheme.ink : MonacoTheme.muted
    }

    private var fill: Color {
        swapFailed ? MonacoTheme.lossWash : MonacoTheme.surfaceSunken
    }
}
