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

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    static let height: CGFloat = 50
    static let strokeWidth: CGFloat = 2

    static func spokenLabel(side: VoteSide, count: Int, isCommitted: Bool) -> String {
        let votes = count == 1 ? "1 vote" : "\(count) votes"
        return isCommitted ? "Voted \(side.title), \(votes)" : "\(side.title), \(votes)"
    }

    private var shape: RoundedRectangle {
        RoundedRectangle(cornerRadius: MonacoTheme.Radius.button, style: .continuous)
    }

    private var progress: Double? {
        guard case .holding(let value) = phase else { return nil }
        return min(max(value, 0), 1)
    }

    private var isCommitted: Bool { phase == .committed }

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
            if isCommitted {
                Label("Voted \(side.title)", systemImage: "checkmark.seal.fill")
            } else {
                Label(side.title, systemImage: side.symbol)
            }
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
            shape.fill(isCommitted ? MonacoTheme.brandFill : MonacoTheme.surfaceSunken)
            if let progress {
                sweep(progress)
            }
            labelRow(labelColor)
            if let progress {
                labelRow(MonacoTheme.onBrand)
                    .mask(alignment: .leading) {
                        GeometryReader { geometry in
                            Rectangle().frame(width: geometry.size.width * progress)
                        }
                    }
            }
        }
        .frame(minHeight: Self.height)
        .overlay {
            if phase == .picked {
                shape.strokeBorder(MonacoTheme.tint, lineWidth: Self.strokeWidth)
            }
        }
        .contentShape(shape)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spokenLabel(side: side, count: count, isCommitted: isCommitted))
        .accessibilityAddTraits(phase == .picked || isCommitted ? .isSelected : [])
    }

    private func sweep(_ progress: Double) -> some View {
        GeometryReader { geometry in
            ZStack(alignment: .topLeading) {
                Rectangle()
                    .fill(MonacoTheme.brandFill)
                    .frame(width: geometry.size.width * progress)
                if !reduceMotion {
                    shape.strokeBorder(MonacoTheme.tint, lineWidth: Self.strokeWidth)
                        .mask(alignment: .topLeading) {
                            Rectangle().frame(
                                width: geometry.size.width * progress, height: MonacoTheme.Radius.button)
                        }
                }
            }
        }
        .clipShape(shape)
    }
}
