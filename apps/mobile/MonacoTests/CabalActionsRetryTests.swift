import Foundation
import MonacoAPI
import MonacoCore
import Observation
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct CabalActionsRetryTests {
    @Test(.timeLimit(.minutes(1)))
    func joiningInPlaceShowsTheMemberActions() async throws {
        let transport = StubTransport(scripted: [
            try Self.reply(.sample(role: nil)),
            Self.noUnread,
            try Self.reply(.sample(role: "member", canVote: true)),
            Self.noUnread,
        ])
        let model = CabalActionsModel(
            cabalID: "cabal-1",
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintSource()
        )
        let tick = RetryTick()
        let window = try Self.window(hosting: RetryHost(tick: tick, model: model))
        defer { window.isHidden = true }
        await Self.until { model.actions == .hidden }
        await transport.waitForRequests(2)

        tick.value += 1

        await transport.waitForRequests(4)
        let paths = await transport.sent.map { $0.path ?? "?" }
        #expect(
            paths == ["/v1/cabals/cabal-1", "/v1/me/cabals", "/v1/cabals/cabal-1", "/v1/me/cabals"],
            "requests sent: \(paths), actions = \(model.actions)")
        await Self.until { model.actions == .member(canPropose: true) }
    }

    @Test(.timeLimit(.minutes(1)))
    func aFailedReadShowsTryAgainAndTryAgainBringsTheActionsBack() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.notConnectedToInternet)),
            Self.noUnread,
            try Self.reply(.sample(role: "member", canVote: true)),
            Self.noUnread,
        ])
        let model = CabalActionsModel(
            cabalID: "cabal-1",
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintSource()
        )
        try Self.setAutomation(enabled: true)
        defer { try? Self.setAutomation(enabled: false) }
        let window = try Self.window(hosting: RetryHost(tick: RetryTick(), model: model))
        defer { window.isHidden = true }

        await Self.until { if case .failed = model.actions { true } else { false } }
        #expect(await Self.element("cabal-actions-failed", in: window) != nil, "no error row")
        #expect(await Self.element("cabal-action-fund", in: window) == nil, "buttons still shown")

        let tryAgain = try #require(await Self.element("cabal-actions-retry", in: window), "no Try again")
        #expect(tryAgain.accessibilityActivate())

        await Self.until { model.actions == .member(canPropose: true) }
        #expect(await Self.element("cabal-action-fund", in: window) != nil, "buttons did not come back")
    }

    private static let noUnread = StubTransport.Reply.response(
        status: .ok, contentType: "application/json", body: Data("[]".utf8))

    private static func reply(_ cabal: Components.Schemas.Cabal) throws -> StubTransport.Reply {
        .response(status: .ok, contentType: "application/json", body: try JSONEncoder().encode(cabal))
    }

    private static func window(hosting view: RetryHost) throws -> UIWindow {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let environment = AppEnvironment(
            auth: auth, hints: FakeHintSource(), isAuthenticated: { true }, endAuthSession: {})
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack { ScrollView { VStack { ForEach([0], id: \.self) { _ in AnyView(view) } } } }
                .environment(environment)
                .environment(ToastCenter())
        )
        window.makeKeyAndVisible()
        return window
    }

    private static func setAutomation(enabled: Bool) throws {
        let library = try #require(dlopen("/usr/lib/libAccessibility.dylib", RTLD_NOW))
        let symbol = try #require(dlsym(library, "_AXSSetAutomationEnabled"))
        unsafeBitCast(symbol, to: (@convention(c) (Int32) -> Void).self)(enabled ? 1 : 0)
    }

    private static func element(_ identifier: String, in root: NSObject, depth: Int = 0) async -> NSObject? {
        for _ in 0..<200 {
            if let found = find(identifier, in: root, depth: depth) { return found }
            await Task.yield()
        }
        return nil
    }

    private static func find(_ identifier: String, in root: NSObject, depth: Int) -> NSObject? {
        if root.responds(to: #selector(getter: UIAccessibilityIdentification.accessibilityIdentifier)),
            root.value(forKey: "accessibilityIdentifier") as? String == identifier
        {
            return root
        }
        guard depth < 80 else { return nil }
        var children: [NSObject] = (root as? UIView)?.subviews ?? []
        if let elements = root.accessibilityElements as? [NSObject] {
            children += elements
        } else {
            let count = root.accessibilityElementCount()
            if count != NSNotFound, count > 0 {
                children += (0..<count).compactMap { root.accessibilityElement(at: $0) as? NSObject }
            }
        }
        return children.lazy.compactMap { find(identifier, in: $0, depth: depth + 1) }.first
    }

    private static func until(_ predicate: () -> Bool) async {
        while !predicate() {
            await withCheckedContinuation { continuation in
                withObservationTracking {
                    _ = predicate()
                } onChange: {
                    continuation.resume()
                }
            }
        }
    }
}

@Observable
@MainActor
private final class RetryTick {
    var value = 0
}

private struct RetryHost: View {
    let tick: RetryTick
    let model: CabalActionsModel

    var body: some View {
        CabalActionsLive(cabalID: model.cabalID, model: model)
            .environment(\.cabalRetry, CabalRetry(tick: tick.value) { tick.value += 1 })
    }
}
