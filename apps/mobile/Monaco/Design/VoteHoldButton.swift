import SwiftUI

enum VoteHoldPhase: Equatable {
    case idle
    case picked
    case dimmed
    case holding(progress: Double)
    case committed
    case disabled
}

struct VoteHoldButton: View {
    let side: VoteSide
    let count: Int
    let phase: VoteHoldPhase

    static let height: CGFloat = 44

    static func spokenLabel(side: VoteSide, count: Int, isCommitted: Bool) -> String {
        let votes = count == 1 ? "1 vote" : "\(count) votes"
        return isCommitted ? "Voted \(side.title), \(votes)" : "\(side.title), \(votes)"
    }

    static func sweepWidth(progress: Double, width: CGFloat) -> CGFloat {
        width * min(max(progress, 0), 1)
    }

    private var shape: Capsule { Capsule(style: .continuous) }

    private var progress: Double? {
        guard case .holding(let value) = phase else { return nil }
        return min(max(value, 0), 1)
    }

    private var isCommitted: Bool { phase == .committed }

    private var baseFill: Color {
        switch phase {
        case .committed: return MonacoTheme.brandFill
        case .dimmed: return MonacoTheme.surfaceSunken.opacity(0.55)
        case .idle, .picked, .holding, .disabled: return MonacoTheme.surfaceSunken
        }
    }

    private var labelColor: Color {
        switch phase {
        case .committed: return MonacoTheme.onBrand
        case .dimmed: return MonacoTheme.secondaryText
        case .disabled: return MonacoTheme.disabledLabel
        case .idle, .picked, .holding: return MonacoTheme.primaryText
        }
    }

    private func labelRow(_ color: Color) -> some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Label(side.title, systemImage: side.symbol)
            Text("\(count)").monospacedDigit()
        }
        .font(MonacoTheme.Typo.button)
        .foregroundStyle(color)
        .lineLimit(1)
        .fixedSize(horizontal: true, vertical: false)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    var body: some View {
        ZStack {
            shape.fill(baseFill)
            if phase == .picked {
                shape.fill(MonacoTheme.brandFill.opacity(0.35))
            }
            if let progress {
                sweep(progress)
            }
            labelRow(labelColor)
            if let progress {
                labelRow(MonacoTheme.onBrand)
                    .mask(alignment: .leading) {
                        GeometryReader { geometry in
                            Rectangle().frame(
                                width: Self.sweepWidth(progress: progress, width: geometry.size.width))
                        }
                    }
            }
        }
        .frame(minHeight: Self.height)
        .contentShape(shape)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spokenLabel(side: side, count: count, isCommitted: isCommitted))
        .accessibilityAddTraits(phase == .picked || isCommitted ? .isSelected : [])
    }

    private func sweep(_ progress: Double) -> some View {
        GeometryReader { geometry in
            ZStack(alignment: .leading) {
                Capsule(style: .continuous)
                    .fill(MonacoTheme.brandFill)
                    .frame(width: Self.sweepWidth(progress: progress, width: geometry.size.width))
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .clipShape(shape)
    }
}
