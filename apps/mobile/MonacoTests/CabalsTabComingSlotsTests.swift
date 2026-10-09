import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct CabalsTabComingSlotsTests {
    @Test func bothSlotsAreLive() {
        #expect(CabalsValueChartSlot.isLive)
        #expect(CabalsBoardSlot.isLive)
    }

    @Test func returnSlotShowsOnlyOnceTheAnswerIsIn() {
        #expect(!CabalsValueChartSlot.shows(.hidden))
        #expect(!CabalsValueChartSlot.shows(.loading))
        #expect(CabalsValueChartSlot.shows(.failed))
        #expect(CabalsValueChartSlot.shows(.loaded))
    }

    @Test(.timeLimit(.minutes(1)))
    func emptyListOffersToStartACabal() async throws {
        let labels = try await Self.labels(restricted: false, body: "[]", until: "No cabals yet")

        #expect(labels.contains("Start a cabal"))
        #expect(labels.contains("Start one with friends, or search above to find one."))
    }

    @Test(.timeLimit(.minutes(1)))
    func restrictedEmptyListHasNoActionToStartACabal() async throws {
        let labels = try await Self.labels(restricted: true, body: "[]", until: "No cabals yet")

        #expect(!labels.contains("Start a cabal"))
        #expect(labels.contains("Search above to find one."))
    }

    @Test(.timeLimit(.minutes(1)))
    func listShowsTheNewCabalCardOnlyToAnUnrestrictedMember() async throws {
        let open = try await Self.labels(restricted: false, body: Self.oneCabal, until: "Weekend pot")
        let restricted = try await Self.labels(restricted: true, body: Self.oneCabal, until: "Weekend pot")

        #expect(open.contains("New cabal"))
        #expect(!restricted.contains("New cabal"))
    }

    private static let oneCabal = """
        [{"id":"01890a5d-ac96-774b-bcce-b302099a8060","name":"Weekend pot","picture_url":null,"role":"creator","can_vote":true,"member_count":1,"joined_at":"2026-10-02T15:00:00Z","pending_request_count":0,"unread_count":0}]
        """

    private static func labels(restricted: Bool, body: String, until marker: String) async throws -> [String] {
        let transport = StubTransport(.json(.ok, body))
        let model = MonacoCore.CabalsTabModel(
            api: APIClient(
                serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport))
        await model.load()
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        setAccessibilityAutomation(true)
        defer { setAccessibilityAutomation(false) }
        let window = UIWindow(windowScene: scene)
        window.frame = CGRect(x: 0, y: 0, width: 390, height: 800)
        window.rootViewController = UIHostingController(
            rootView: MyCabalsContent(model: model, standings: [:], open: { _ in }, create: {})
                .environment(\.accountRestricted, restricted)
        )
        window.makeKeyAndVisible()
        defer { window.isHidden = true }
        window.layoutIfNeeded()
        var labels: [String] = []
        var settled = false
        while !settled {
            try Task.checkCancellation()
            await withCheckedContinuation { continuation in
                RunLoop.main.perform { continuation.resume() }
            }
            let next = accessibilityLabels(of: window)
            settled = next == labels && next.contains { $0.contains(marker) }
            labels = next
        }
        return labels
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
