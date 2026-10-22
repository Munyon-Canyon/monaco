import SwiftUI

enum VoteBarMetrics {
    static let barHeight: CGFloat = 8
    static let gap: CGFloat = 3
}

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

struct VoteBar: View {
    let yes: Int
    let no: Int
    let notVoted: Int
    let needed: Int
    let isClosed: Bool

    private var headline: String {
        if isClosed { return "Voting closed" }
        return needed == 1 ? "1 yes vote to pass" : "\(needed) yes votes to pass"
    }

    private var tally: String { "\(yes) yes · \(no) no · \(notVoted) not voted" }

    static func weights(yes: Int, no: Int, notVoted: Int) -> [Double] {
        let counts = [yes, no, notVoted].map { Double(max($0, 0)) }
        return counts.reduce(0, +) > 0 ? counts : [0, 0, 1]
    }

    private var segments: [SegmentedBar.Segment] {
        let colors = [MonacoTheme.profitVivid, MonacoTheme.lossVivid, MonacoTheme.border]
        return zip(Self.weights(yes: yes, no: no, notVoted: notVoted), colors).enumerated().map {
            SegmentedBar.Segment(id: $0.offset, weight: $0.element.0, color: $0.element.1)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(headline)
            SegmentedBar(segments: segments, height: VoteBarMetrics.barHeight, gap: VoteBarMetrics.gap)
                .monacoDrawOn()
                .padding(.top, MonacoTheme.Space.sm)
            Text(tally)
                .padding(.top, MonacoTheme.Space.s)
        }
        .font(MonacoTheme.Typo.caption)
        .foregroundStyle(MonacoTheme.secondaryText)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(yes) yes, \(no) no, \(notVoted) not voted. \(headline).")
    }
}
