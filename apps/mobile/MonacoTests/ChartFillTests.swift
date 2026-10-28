import SwiftUI
import Testing

@testable import Monaco

@Suite(.timeLimit(.minutes(1)))
struct ChartFillTests {
    private static let xs: [Double] = [0, 40, 80, 120, 160]
    private static let ys: [Double] = [60, 30, 45, 10, 25]
    private static let baseline = 100.0

    private static func points(of path: Path) -> [CGPoint] {
        var points: [CGPoint] = []
        path.cgPath.applyWithBlock { pointer in
            let element = pointer.pointee
            switch element.type {
            case .moveToPoint, .addLineToPoint:
                points.append(element.points[0])
            case .addQuadCurveToPoint:
                points.append(element.points[1])
            case .addCurveToPoint:
                points.append(element.points[2])
            case .closeSubpath:
                break
            @unknown default:
                break
            }
        }
        return points
    }

    @Test func fillOpacityIsTenPercent() {
        #expect(MonacoLineStyle.fillOpacity == 0.10)
    }

    @Test func areaClosesOnTheBaseline() throws {
        let area = try #require(MonacoLineStyle.area(xs: Self.xs, ys: Self.ys, baseline: Self.baseline))
        let points = Self.points(of: area)

        #expect(points.first?.y == CGFloat(Self.baseline))
        #expect(points.last?.y == CGFloat(Self.baseline))
        #expect(points.first?.x == Self.xs.first.map { CGFloat($0) })
        #expect(points.last?.x == Self.xs.last.map { CGFloat($0) })
    }

    @Test func areaFollowsTheLineAlongTheTop() throws {
        let line = try #require(MonacoLineStyle.line(xs: Self.xs, ys: Self.ys))
        let area = try #require(MonacoLineStyle.area(xs: Self.xs, ys: Self.ys, baseline: Self.baseline))
        let top = Array(Self.points(of: area).dropFirst().dropLast())

        #expect(top == Self.points(of: line))
    }

    @Test func areaNeedsTwoPoints() {
        #expect(MonacoLineStyle.area(xs: [0], ys: [1], baseline: Self.baseline) == nil)
        #expect(MonacoLineStyle.area(xs: [], ys: [], baseline: Self.baseline) == nil)
    }
}
