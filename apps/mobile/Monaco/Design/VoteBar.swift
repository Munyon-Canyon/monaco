import SwiftUI

struct SegmentedBar: View {
    struct Segment: Identifiable, Equatable {
        let id: Int
        let weight: Double
        let color: Color
    }

    let segments: [Segment]
    let height: CGFloat
    let gap: CGFloat

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    static func widths(_ weights: [Double], in total: CGFloat, gap: CGFloat) -> [CGFloat] {
        let drawn = weights.map { max($0, 0) }
        let sum = drawn.reduce(0, +)
        guard sum > 0, total > 0 else { return drawn.map { _ in 0 } }
        let room = max(total - gap * CGFloat(drawn.filter { $0 > 0 }.count - 1), 0)
        return drawn.map { CGFloat($0 / sum) * room }
    }

    var body: some View {
        GeometryReader { geometry in
            let widths = Self.widths(segments.map(\.weight), in: geometry.size.width, gap: gap)
            HStack(spacing: gap) {
                ForEach(segments.indices.filter { widths[$0] > 0 }, id: \.self) { index in
                    Rectangle()
                        .fill(segments[index].color)
                        .frame(width: widths[index])
                }
            }
        }
        .frame(height: height)
        .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: segments)
    }
}

struct VoteTallyBar: View {
    let yes: Int
    let no: Int
    let eligible: Int
    var winner: VoteSide? = nil

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    static let height: CGFloat = 4

    static func fractions(yes: Int, no: Int, eligible: Int) -> (yes: Double, no: Double) {
        guard eligible > 0 else { return (0, 0) }
        let yesShare = Double(max(yes, 0)) / Double(eligible)
        let noShare = Double(max(no, 0)) / Double(eligible)
        let total = yesShare + noShare
        guard total > 1 else { return (yesShare, noShare) }
        return (yesShare / total, noShare / total)
    }

    static func fill(_ side: VoteSide, winner: VoteSide?) -> Color {
        winner == side ? MonacoTheme.primaryText : MonacoTheme.secondaryText
    }

    private var shares: (yes: Double, no: Double) { Self.fractions(yes: yes, no: no, eligible: eligible) }

    var body: some View {
        GeometryReader { geometry in
            ZStack(alignment: .leading) {
                Rectangle().fill(MonacoTheme.surfaceSunken)
                Rectangle()
                    .fill(Self.fill(.yes, winner: winner))
                    .frame(width: geometry.size.width * shares.yes)
                Rectangle()
                    .fill(Self.fill(.no, winner: winner))
                    .frame(width: geometry.size.width * shares.no)
                    .frame(maxWidth: .infinity, alignment: .trailing)
            }
        }
        .frame(height: Self.height)
        .clipShape(Capsule())
        .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: shares.yes)
        .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: shares.no)
        .accessibilityHidden(true)
    }
}
