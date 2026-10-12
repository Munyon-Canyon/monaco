import XCTest

@testable import MonacoCore

final class ChartCurveTests: XCTestCase {
    private func sample(_ segment: ChartCurve.Segment, from start: (x: Double, y: Double), at t: Double) -> Double {
        let u = 1 - t
        return u * u * u * start.y + 3 * u * u * t * segment.control1.y + 3 * u * t * t * segment.control2.y
            + t * t * t * segment.end.y
    }

    func testMonotone_needsTwoPoints() {
        XCTAssertTrue(ChartCurve.monotone(xs: [0], ys: [1]).isEmpty)
        XCTAssertEqual(ChartCurve.monotone(xs: [0, 1], ys: [1, 2]).count, 1)
    }

    func testMonotone_endsExactlyOnEveryPoint() {
        let xs = [0.0, 10, 25, 30]
        let ys = [5.0, 9, 2, 2]

        let segments = ChartCurve.monotone(xs: xs, ys: ys)

        XCTAssertEqual(segments.map(\.end.x), [10, 25, 30])
        XCTAssertEqual(segments.map(\.end.y), [9, 2, 2])
    }

    func testMonotone_neverOvershootsASpikeOrAFlatRun() {
        let xs = (0..<9).map { Double($0) * 10 }
        let ys = [10.0, 10, 10, 30, 10, 10, 10, 11, 11]
        let segments = ChartCurve.monotone(xs: xs, ys: ys)
        var start = (x: xs[0], y: ys[0])
        for (index, segment) in segments.enumerated() {
            let low = min(ys[index], ys[index + 1])
            let high = max(ys[index], ys[index + 1])
            for step in 0...20 {
                let y = sample(segment, from: start, at: Double(step) / 20)
                XCTAssertGreaterThanOrEqual(y, low - 1e-9, "segment \(index) dips below its ends")
                XCTAssertLessThanOrEqual(y, high + 1e-9, "segment \(index) rises above its ends")
            }
            start = segment.end
        }
    }

    func testMonotone_aFlatSegmentStaysFlat() {
        let segments = ChartCurve.monotone(xs: [0, 1, 2], ys: [3, 3, 3])

        XCTAssertTrue(segments.allSatisfy { $0.control1.y == 3 && $0.control2.y == 3 })
    }

    func testMonotone_handlesRepeatedXWithoutNaN() {
        let segments = ChartCurve.monotone(xs: [0, 0, 1], ys: [1, 2, 3])

        XCTAssertTrue(segments.allSatisfy { $0.control1.y.isFinite && $0.control2.y.isFinite })
    }
}
