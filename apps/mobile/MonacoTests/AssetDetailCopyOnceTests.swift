import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
@Suite(.serialized, .timeLimit(.minutes(1)))
struct AssetDetailCopyOnceTests {
    private static let solanaLine = "Trading 24/7 on Solana"
    private static let historyLine = "Price history builds up over time."

    @Test func closedMarketWithOnePricePointShowsEachLineOnce() async throws {
        let labels = try await Self.screenLabels(session: .closed, points: 1)

        #expect(Self.count(of: Self.solanaLine, in: labels) == 1)
        #expect(Self.count(of: Self.historyLine, in: labels) == 1)
    }

    @Test func openMarketHasNoSolanaLine() async throws {
        let labels = try await Self.screenLabels(session: .open, points: 2)

        #expect(Self.count(of: Self.solanaLine, in: labels) == 0)
    }

    private static func count(of text: String, in labels: [String]) -> Int {
        labels.reduce(0) { $0 + $1.components(separatedBy: text).count - 1 }
    }

    private static func screenLabels(session: Components.Schemas.MarketSession, points: Int) async throws -> [String] {
        var detail = Components.Schemas.AssetDetail.googl
        detail.session = session
        let series = AssetChartSeries(
            range: .oneDay,
            points: (0..<points).map {
                .init(timestamp: 1_772_596_200 + Int64($0) * 600, priceUsdcMicros: 174_000_000)
            }
        )
        let model = AssetDetailClientModel(sampleDetail: detail, chart: series)
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let environment = AppEnvironment(
            auth: auth, hints: FakeHintSource(), isAuthenticated: { true }, endAuthSession: {})
        setAccessibilityAutomation(true)
        defer { setAccessibilityAutomation(false) }
        let window = UIWindow(windowScene: scene)
        window.frame = CGRect(x: 0, y: 0, width: 390, height: 1600)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack { AssetDetailClientView(symbol: detail.symbol, model: model) }
                .environment(environment)
                .environment(ToastCenter())
        )
        window.makeKeyAndVisible()
        defer { window.isHidden = true }
        window.layoutIfNeeded()
        var labels: [String] = []
        var settled = false
        while !settled {
            try Task.checkCancellation()
            await nextRunLoopTurn()
            let next = accessibilityLabels(of: window)
            settled = next == labels && next.contains { $0.contains(detail.attribution) }
            labels = next
        }
        return labels
    }

    private static func nextRunLoopTurn() async {
        await withCheckedContinuation { continuation in
            RunLoop.main.perform { continuation.resume() }
        }
    }

    private static func setAccessibilityAutomation(_ enabled: Bool) {
        typealias SetAutomation = @convention(c) (Bool) -> Void
        guard let handle = dlopen("/usr/lib/libAccessibility.dylib", RTLD_NOW),
            let symbol = dlsym(handle, "_AXSSetAutomationEnabled")
        else { return }
        unsafeBitCast(symbol, to: SetAutomation.self)(enabled)
    }

    private static func accessibilityLabels(of root: NSObject) -> [String] {
        var labels: [String] = []
        if let label = root.accessibilityLabel, !label.isEmpty { labels.append(label) }
        if let children = accessibilityChildren(of: root) {
            for child in children { labels += accessibilityLabels(of: child) }
        } else if let view = root as? UIView {
            for subview in view.subviews { labels += accessibilityLabels(of: subview) }
        }
        return labels
    }

    private static func accessibilityChildren(of node: NSObject) -> [NSObject]? {
        if let elements = node.accessibilityElements { return elements.compactMap { $0 as? NSObject } }
        let count = node.accessibilityElementCount()
        guard count != NSNotFound, count > 0 else { return nil }
        return (0..<count).compactMap { node.accessibilityElement(at: $0) as? NSObject }
    }
}
