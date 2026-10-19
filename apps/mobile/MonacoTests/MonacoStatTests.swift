import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct MonacoStatTests {
    private static let six: [StatItem] = [
        .init(label: "Units", value: "30.0000"),
        .init(label: "Average buy", value: "$412.40"),
        .init(label: "Cost", value: "$12,372.00"),
        .init(label: "Value", value: "$13,950.00"),
        .init(label: "Unrealized", value: "+$1,578.00"),
        .init(label: "Realized", value: "$0.00"),
    ]

    private static func height(of view: some View, size: DynamicTypeSize = .large) -> CGFloat {
        UIHostingController(rootView: view.environment(\.dynamicTypeSize, size))
            .sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude)).height
    }

    private static func strip() -> MoneyStrip {
        MoneyStrip(netWorth: "$11,582.32", cash: "$500.00", depositTitle: "Deposit", onDeposit: {})
    }

    @Test func itemsFillRowsOfTheColumnCount() {
        #expect(StatGrid.rows(Self.six, columns: 2).map(\.count) == [2, 2, 2])
        #expect(StatGrid.rows(Array(Self.six.prefix(5)), columns: 2).map(\.count) == [2, 2, 1])
        #expect(StatGrid.rows(Self.six, columns: 1).map(\.count) == [1, 1, 1, 1, 1, 1])
    }

    @Test func eachCellIsAtLeastSeventyTwoTallAndTheGridHasTwoColumnsUntilAccessibilitySizes() {
        let cell = StatMetrics.cellMinHeight
        let normal = Self.height(of: StatGrid(items: Self.six))
        let accessible = Self.height(of: StatGrid(items: Self.six), size: .accessibility3)

        #expect(normal >= 3 * cell && normal < 6 * cell, "three rows of two: \(normal)")
        #expect(accessible >= 6 * cell, "six rows of one: \(accessible)")
    }

    @Test func eachGridCellReadsAsOneElement() async throws {
        let labels = try await RenderedView.labels(StatGrid(items: Self.six)) { $0.contains("Units, 30.0000") }

        #expect(labels.contains("Units, 30.0000"), "labels: \(labels)")
        #expect(labels.contains("Cost, $12,372.00"), "labels: \(labels)")
        #expect(!labels.contains("Units"), "labels: \(labels)")
    }

    @Test func aPairSitsSideBySideUntilAccessibilitySizesThenStacks() {
        let one = [StatItem(label: "Your share", value: "$7,000.00")]
        let two = one + [StatItem(label: "Cash", value: "$5,100.00")]

        #expect(Self.height(of: StatPair(items: two)) == Self.height(of: StatPair(items: one)))
        #expect(
            Self.height(of: StatPair(items: two), size: .accessibility3)
                > Self.height(of: StatPair(items: one), size: .accessibility3))
    }

    @Test func aPairBetweenTwoRulesHoldsASixteenPointPaddedCard() {
        let one = [StatItem(label: "Your share", value: "$7,000.00")]
        let cellOnly = Self.height(
            of: VStack(alignment: .leading, spacing: 0) {
                Text("Your share").font(MonacoTheme.Typo.caption)
                Text("$7,000.00").moneyFont(.card)
            })

        #expect(Self.height(of: StatPair(items: one)) > cellOnly + 2 * MonacoTheme.Space.m)
    }

    @Test func pairCellsReadAsOneElement() async throws {
        let items = [StatItem(label: "Your share", value: "$7,000.00"), StatItem(label: "Cash", value: "$5,100.00")]

        let labels = try await RenderedView.labels(StatPair(items: items)) { $0.contains("Cash, $5,100.00") }

        #expect(labels.contains("Your share, $7,000.00"), "labels: \(labels)")
        #expect(labels.contains("Cash, $5,100.00"), "labels: \(labels)")
    }

    @Test func theStripStacksTheMetricsAndTheDepositButtonAtAccessibilitySizes() {
        let normal = Self.height(of: Self.strip())
        let accessible = Self.height(of: Self.strip(), size: .accessibility3)

        #expect(accessible > normal + 44, "normal \(normal), accessible \(accessible)")
    }

    @Test func theStripKeepsTheDepositHitAreaAtLeastFortyFourBySeventyEight() async throws {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let scene = try #require(UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first)
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(rootView: Self.strip())
        window.makeKeyAndVisible()
        defer { window.isHidden = true }
        await ProposalTestWindow.until { Self.frame(ofLabel: "Deposit", in: window) != nil }

        let frame = try #require(Self.frame(ofLabel: "Deposit", in: window))

        #expect(frame.height >= 44 && frame.width >= 78, "deposit hit area \(frame)")
    }

    @Test func theStripMetricsReadAsOneElementEach() async throws {
        let labels = try await RenderedView.labels(Self.strip()) { $0.contains("Cash, $500.00") }

        #expect(labels.contains("Net worth, $11,582.32"), "labels: \(labels)")
        #expect(labels.contains("Cash, $500.00"), "labels: \(labels)")
        #expect(labels.contains("Deposit"), "labels: \(labels)")
    }

    private static func frame(ofLabel label: String, in root: NSObject, depth: Int = 0) -> CGRect? {
        guard depth < 80 else { return nil }
        if root.accessibilityLabel == label { return root.accessibilityFrame }
        var children: [NSObject] = (root as? UIView)?.subviews ?? []
        if let elements = root.accessibilityElements as? [NSObject] {
            children += elements
        } else {
            let count = root.accessibilityElementCount()
            if count != NSNotFound, count > 0 {
                children += (0..<count).compactMap { root.accessibilityElement(at: $0) as? NSObject }
            }
        }
        return children.lazy.compactMap { frame(ofLabel: label, in: $0, depth: depth + 1) }.first
    }
}
