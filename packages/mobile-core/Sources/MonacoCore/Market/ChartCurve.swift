public enum ChartCurve {
    public struct Segment: Equatable, Sendable {
        public let control1: (x: Double, y: Double)
        public let control2: (x: Double, y: Double)
        public let end: (x: Double, y: Double)

        public static func == (lhs: Segment, rhs: Segment) -> Bool {
            lhs.control1 == rhs.control1 && lhs.control2 == rhs.control2 && lhs.end == rhs.end
        }
    }

    public static func monotone(xs: [Double], ys: [Double]) -> [Segment] {
        let count = min(xs.count, ys.count)
        guard count >= 2 else { return [] }
        var widths: [Double] = []
        var slopes: [Double] = []
        for index in 0..<(count - 1) {
            let width = xs[index + 1] - xs[index]
            widths.append(width)
            slopes.append(width == 0 ? 0 : (ys[index + 1] - ys[index]) / width)
        }
        var tangents = [Double](repeating: 0, count: count)
        tangents[0] = slopes[0]
        tangents[count - 1] = slopes[count - 2]
        if count > 2 {
            for index in 1..<(count - 1) {
                let before = slopes[index - 1]
                let after = slopes[index]
                tangents[index] = before * after <= 0 ? 0 : (before + after) / 2
            }
        }
        for index in 0..<(count - 1) {
            let slope = slopes[index]
            if slope == 0 {
                tangents[index] = 0
                tangents[index + 1] = 0
                continue
            }
            let alpha = tangents[index] / slope
            let beta = tangents[index + 1] / slope
            let magnitude = (alpha * alpha + beta * beta).squareRoot()
            if magnitude > 3 {
                let scale = 3 / magnitude
                tangents[index] = scale * alpha * slope
                tangents[index + 1] = scale * beta * slope
            }
        }
        return (0..<(count - 1)).map { index in
            let third = widths[index] / 3
            return Segment(
                control1: (xs[index] + third, ys[index] + tangents[index] * third),
                control2: (xs[index + 1] - third, ys[index + 1] - tangents[index + 1] * third),
                end: (xs[index + 1], ys[index + 1])
            )
        }
    }
}
