import Foundation
import MonacoAPI
import MonacoCore
import Observation
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct StocksTabLoadTests {
    @Test(.timeLimit(.minutes(1)))
    func theStocksTabAsksForAssetsBeforeItHasAModel() async throws {
        let transport = StubTransport(.json(.ok, #"{"assets":[],"next_cursor":null}"#))
        let model = MonacoCore.StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintSource(),
            clock: ContinuousClock()
        )
        let window = try Self.window(hosting: StocksTabView(makeModel: { _ in model }))
        defer { window.isHidden = true }

        await Self.until { model.all.phase != .idle && model.all.phase != .loading }

        let paths = await transport.sent.map { URLComponents(string: $0.path ?? "")?.path }
        #expect(paths.first == "/v1/assets")
    }

    private static func window(hosting view: StocksTabView) throws -> UIWindow {
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
