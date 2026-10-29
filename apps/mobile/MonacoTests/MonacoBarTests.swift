import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct MonacoBarTests {
    private static func height(of view: some View, size: DynamicTypeSize = .large) -> CGFloat {
        UIHostingController(rootView: view.environment(\.dynamicTypeSize, size))
            .sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude)).height
    }

    private static let slices: [AllocationBar.Slice] = [
        .init(label: "NVDA", fraction: 0.4),
        .init(label: "MSFT", fraction: 0.25),
        .init(label: "Cash", fraction: 0.35, isCash: true),
    ]

    @Test func segmentsShareTheWidthByWeightAndKeepTheirGaps() {
        let widths = SegmentedBar.widths([3, 1, 4], in: 350, gap: 3)

        let total: CGFloat = widths.reduce(0, +) + 6
        let first: CGFloat = widths[0] / widths[1]
        let third: CGFloat = widths[2] / widths[1]
        #expect(abs(total - 350) < 0.001)
        #expect(abs(first - 3) < 0.001)
        #expect(abs(third - 4) < 0.001)
    }

    @Test func aSegmentWithNoWeightTakesNoWidthAndNoGap() {
        let widths = SegmentedBar.widths([3, 0, 1], in: 100, gap: 3)

        #expect(widths[1] == 0)
        let total: CGFloat = widths[0] + widths[2] + 3
        #expect(abs(total - 100) < 0.001)
    }

    @Test func aBarWithNoWeightDrawsNothing() {
        #expect(SegmentedBar.widths([0, 0, 0], in: 100, gap: 3) == [0, 0, 0])
    }

    @Test func holdingsTakeTheTintsInOrderAndReuseTheLastHoldingTint() {
        let holdings = (0..<5).map { AllocationBar.Slice(label: "S\($0)", fraction: 0.1) }

        #expect(AllocationBar.tintIndexes(for: holdings) == [0, 1, 2, 2, 2])
    }

    @Test func cashAlwaysTakesTheLastTintWhereverItSits() {
        let slices: [AllocationBar.Slice] = [
            .init(label: "NVDA", fraction: 0.4),
            .init(label: "Cash", fraction: 0.2, isCash: true),
            .init(label: "MSFT", fraction: 0.4),
        ]

        #expect(AllocationBar.tintIndexes(for: slices) == [0, 3, 1])
        #expect(MonacoTheme.allocationTints.count == 4)
    }

    @Test func theAllocationBarReadsAsOneElementWithTheListedWording() async throws {
        let labels = try await RenderedView.labels(AllocationBar(slices: Self.slices)) {
            $0.contains { $0.hasPrefix("Allocation") }
        }

        #expect(
            Set(labels) == ["Allocation: NVDA 40 percent, MSFT 25 percent, Cash 35 percent."], "labels: \(labels)")
    }

    @Test func theBarAloneIsSevenPointsAndTheLegendWrapsAtTheLargestTextSize() {
        let bare = Self.height(of: AllocationBar(slices: Self.slices, showsLegend: false))
        let legend = Self.height(of: AllocationBar(slices: Self.slices))
        let large = Self.height(of: AllocationBar(slices: Self.slices), size: .accessibility5)

        #expect(bare == AllocationBarMetrics.barHeight)
        #expect(legend > bare)
        #expect(large > legend * 2)
    }
}
