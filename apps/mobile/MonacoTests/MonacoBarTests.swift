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

        #expect(abs(widths.reduce(0, +) + 2 * 3 - 350) < 0.001)
        #expect(abs(widths[0] / widths[1] - 3) < 0.001)
        #expect(abs(widths[2] / widths[1] - 4) < 0.001)
    }

    @Test func aSegmentWithNoWeightTakesNoWidthAndNoGap() {
        let widths = SegmentedBar.widths([3, 0, 1], in: 100, gap: 3)

        #expect(widths[1] == 0)
        #expect(abs(widths[0] + widths[2] + 3 - 100) < 0.001)
    }

    @Test func aBarWithNoWeightDrawsNothing() {
        #expect(SegmentedBar.widths([0, 0, 0], in: 100, gap: 3) == [0, 0, 0])
    }

    @Test func aTallyKeepsItsCountsAndAnEmptyOneIsAllNotVoted() {
        #expect(VoteBar.weights(yes: 3, no: 1, notVoted: 4) == [3, 1, 4])
        #expect(VoteBar.weights(yes: 0, no: 0, notVoted: 0) == [0, 0, 1])
        #expect(VoteBar.weights(yes: -2, no: 1, notVoted: 0) == [0, 1, 0])
    }

    @Test func theVoteBarReadsAsOneElementWithTheListedWording() async throws {
        let labels = try await RenderedView.labels(
            VoteBar(yes: 3, no: 1, notVoted: 4, needed: 5, isClosed: false)
        ) { $0.contains { $0.contains("yes") } }

        #expect(Set(labels) == ["3 yes, 1 no, 4 not voted. 5 yes votes to pass."], "labels: \(labels)")
    }

    @Test func aClosedVoteSaysSoAndASingleVoteIsNotPlural() async throws {
        let closed = try await RenderedView.labels(
            VoteBar(yes: 5, no: 2, notVoted: 1, needed: 5, isClosed: true)
        ) { $0.contains { $0.contains("yes") } }
        let single = try await RenderedView.labels(
            VoteBar(yes: 0, no: 0, notVoted: 1, needed: 1, isClosed: false)
        ) { $0.contains { $0.contains("yes") } }

        #expect(Set(closed) == ["5 yes, 2 no, 1 not voted. Voting closed."], "labels: \(closed)")
        #expect(Set(single) == ["0 yes, 0 no, 1 not voted. 1 yes vote to pass."], "labels: \(single)")
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
