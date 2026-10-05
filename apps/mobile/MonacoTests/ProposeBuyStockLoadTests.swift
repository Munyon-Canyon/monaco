import Foundation
import MonacoAPI
import MonacoCore
import Observation
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ProposeBuyStockLoadTests {
    @Test(.timeLimit(.minutes(1)))
    func theBuyListAsksForStocksBeforeItHasAnyToShow() async throws {
        let transport = StubTransport(.json(.ok, "[]"))
        let model = MonacoCore.StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintSource(), clock: ContinuousClock())
        let window = try Self.window(hosting: ProposeBuyStockView(cabalID: "cabal-1", makeModel: { _ in model }))
        defer { window.isHidden = true }

        await Self.until { await !transport.sent.isEmpty }

        #expect(await !transport.sent.isEmpty)
    }

    private static func window(hosting view: ProposeBuyStockView) throws -> UIWindow {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let environment = AppEnvironment(
            auth: auth, hints: FakeHintSource(), isAuthenticated: { true }, endAuthSession: {})
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack { view }
                .environment(environment)
                .environment(ToastCenter())
        )
        window.makeKeyAndVisible()
        return window
    }

    private static func until(_ predicate: () async -> Bool) async {
        while !(await predicate()) { await Task.yield() }
    }
}
