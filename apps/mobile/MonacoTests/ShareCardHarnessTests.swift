import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ShareCardHarnessTests {
    private static func shown(_ sample: ShareSampleCase, expecting id: String) async throws -> Set<String> {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try ProposalTestWindow.hosting(ShareSampleHarness(sample: sample))
        defer { TestWindow.close(window) }
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains(id) }
        return Set(AccessibilityTree.identifiers(in: window))
    }

    @Test(arguments: [ShareSampleCase.post, .square, .pnlOff, .buySellOff, .truncated])
    func everyCardCaseRendersAndCarriesShareCard(sample: ShareSampleCase) async throws {
        let ids = try await Self.shown(sample, expecting: "share-card")

        #expect(ids.contains("share-card"), "\(ids)")
    }

    @Test func loadingAndFailureHaveTheirOwnMarkers() async throws {
        #expect(try await Self.shown(.loading, expecting: "share-loading").contains("share-loading"))
        #expect(try await Self.shown(.failed, expecting: "share-failed").contains("share-failed"))
    }

    @Test func theCardReadsAsOneElementWithTheSpokenLine() {
        let spec = ShareSampleData.spec()

        #expect(spec.spoken == "AC Capital, NVDA, long 38 shares, up 1,027 dollars 90, 14 percent")
    }

    @Test func theChartDrawsEveryMarkerAtItsOwnPlace() {
        let spec = ShareSampleData.spec()
        let layout = FShareChart.Layout(chart: spec.chart, size: CGSize(width: 330, height: 130), scale: 1)

        #expect(layout.discs.count == 4)
        #expect(layout.discs.filter { $0.callout != nil }.count == 2)
        let xs = layout.discs.map(\.center.x)
        #expect(xs == xs.sorted())
        #expect(layout.cutoffX(progress: 1) == 330)
    }

    @Test func theCasesMatchTheLaunchArgument() {
        for sample in ShareSampleCase.allCases {
            #expect(ShareSampleCase.matching([ShareSampleCase.launchArgument, sample.rawValue]) == sample)
        }
        #expect(ShareSampleCase.matching(["-Other"]) == nil)
    }
}
