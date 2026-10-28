import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct MonacoControlsTests {
    private static let phoneWidth: CGFloat = 393
    private static let ranges = ["1H", "1D", "1W", "1M", "3M", "6M", "All"]

    private static func fitted(_ view: some View, at size: DynamicTypeSize, width: CGFloat = phoneWidth) -> CGSize {
        UIHostingController(rootView: view.environment(\.dynamicTypeSize, size))
            .sizeThatFits(in: CGSize(width: width, height: CGFloat.greatestFiniteMagnitude))
    }

    private static func ink(_ view: some View) -> (alpha: [[UInt8]], width: Int, height: Int)? {
        let renderer = ImageRenderer(
            content: view.environment(\.colorScheme, .light).environment(\.displayScale, 1))
        renderer.scale = 1
        renderer.proposedSize = ProposedViewSize(width: phoneWidth, height: nil)
        guard let image = renderer.cgImage else { return nil }
        let width = image.width
        let rowCount = image.height
        var pixels = [UInt8](repeating: 0, count: width * rowCount * 4)
        guard
            let context = CGContext(
                data: &pixels, width: width, height: rowCount, bitsPerComponent: 8, bytesPerRow: width * 4,
                space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)
        else { return nil }
        context.draw(image, in: CGRect(x: 0, y: 0, width: width, height: rowCount))
        var rows: [[UInt8]] = []
        for row in 0..<rowCount {
            var alphas: [UInt8] = []
            for column in 0..<width {
                alphas.append(pixels[(row * width + column) * 4 + 3])
            }
            rows.append(alphas)
        }
        return (rows, width, rowCount)
    }

    @Test func theSelectedBarIsAsWideAsItsLabelAndThereIsOneHairline() throws {
        let tabs = MonacoUnderlineTabs(["Rules"], selection: .constant("Rules")) { $0 }
        let drawn = try #require(Self.ink(tabs))
        let barRow = drawn.alpha[drawn.height - 2]
        let barWidth = barRow.filter { $0 > 0 }.count
        let labelWidth = UIHostingController(
            rootView: Text("Rules").font(MonacoTheme.Typo.section).fixedSize()
        )
        .sizeThatFits(in: CGSize(width: 1000, height: 100)).width
        #expect(
            abs(CGFloat(barWidth) - labelWidth) <= 2,
            "bar \(barWidth)pt, label \(labelWidth)pt, \(drawn.width)x\(drawn.height)")
        let fullRows = drawn.alpha.filter { row in row.allSatisfy { $0 > 0 } }
        #expect(fullRows.count == 1, "\(fullRows.count) full-width rows")
    }

    @Test(arguments: DynamicTypeSize.allCases)
    func everyTabAndRangeLabelIsAtLeastFortyFourPointsTall(size: DynamicTypeSize) {
        let tabs = MonacoUnderlineTabs(["Treasury", "Rules"], selection: .constant("Rules")) { $0 }
        #expect(Self.fitted(tabs, at: size).height >= MonacoButtonMetrics.hitTarget)
        let range = MonacoRangeChips(
            ranges: Self.ranges, selection: "1M", identifierPrefix: "r", label: { $0 }, onSelect: { _ in })
        #expect(Self.fitted(range, at: size).height >= MonacoButtonMetrics.hitTarget)
    }

    private struct PeriodProbe: View {
        @State private var period = "Month"

        var body: some View {
            MonacoPeriodLabels(
                title: "This month", dates: "Oct 1 – 31", periods: ["Week", "Month", "All time"], selection: $period,
                identifierPrefix: "p", label: { $0 })
        }
    }

    @Test func thePeriodLabelsWrapUnderTheTitleAtAccessibilitySizesOnly() {
        let large = Self.fitted(PeriodProbe(), at: .large).height
        let accessibility = Self.fitted(PeriodProbe(), at: .accessibility3).height
        #expect(large < 2 * MonacoButtonMetrics.hitTarget, "\(large)pt at large")
        #expect(accessibility >= 2 * MonacoButtonMetrics.hitTarget, "\(accessibility)pt at accessibility3")
    }

    @Test func theRangeChipsKeepTheirIdentifiers() {
        let plain = MonacoRangeChips(
            ranges: Self.ranges, selection: "1M", identifierPrefix: "home-portfolio", label: { $0 },
            onSelect: { _ in })
        #expect(plain.identifier("3M") == "home-portfolio-3M")
        let leaderboard = MonacoRangeChips(
            ranges: LeaderboardRange.allCases, selection: .oneDay, identifierPrefix: "board", onSelect: { _ in })
        #expect(leaderboard.identifier(.oneWeek) == "board-range-1W")
    }

    @Test func aChipBarOfFiveItemsIsOverTheLimit() {
        #expect(MonacoChipBar<String>.fits(4, maxVisible: 4))
        #expect(!MonacoChipBar<String>.fits(5, maxVisible: 4))
    }

    @Test func theOnChipIsBrandFillWithAnOnBrandLabel() {
        let on = MonacoChipLabel.colors(isSelected: true, isEnabled: true)
        #expect(on.fill == MonacoTheme.brandFill)
        #expect(on.label == MonacoTheme.onBrand)
        let off = MonacoChipLabel.colors(isSelected: false, isEnabled: true)
        #expect(off.fill == MonacoTheme.surfaceSunken)
        #expect(off.label == MonacoTheme.primaryText)
        let disabled = MonacoChipLabel.colors(isSelected: true, isEnabled: false)
        #expect(disabled.label == MonacoTheme.disabledLabel)
    }
}
