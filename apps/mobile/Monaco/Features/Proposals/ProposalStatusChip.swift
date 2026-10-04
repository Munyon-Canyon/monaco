import MonacoCore
import SwiftUI

struct ProposalStatusChip: View {
    let label: String
    let status: ProposalStatus
    let swap: SwapState?

    private enum Tone {
        case done
        case moving
        case refused
        case closed
    }

    private var tone: Tone {
        if status == .executionBlocked || swap == .failed { return .refused }
        return switch status {
        case .executed: .done
        case .passed: .moving
        case .failed, .voided: .refused
        case .open, .expired, .withdrawn, .executionBlocked: .closed
        }
    }

    var body: some View {
        Text(label)
            .font(MonacoTheme.Typo.captionStrong)
            .foregroundStyle(foreground)
            .lineLimit(1)
            .padding(.horizontal, 10)
            .padding(.vertical, 4)
            .background(Capsule().fill(fill))
            .accessibilityIdentifier("proposal-status-chip")
    }

    private var foreground: Color {
        switch tone {
        case .done: MonacoTheme.profitOnWash
        case .moving: MonacoTheme.brandOnWash
        case .refused: MonacoTheme.lossOnWash
        case .closed: MonacoTheme.muted
        }
    }

    private var fill: Color {
        switch tone {
        case .done: MonacoTheme.profitWash
        case .moving: MonacoTheme.brandWash
        case .refused: MonacoTheme.lossWash
        case .closed: MonacoTheme.surfaceSunken
        }
    }
}
