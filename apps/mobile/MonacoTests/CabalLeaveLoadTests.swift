import Foundation
import MonacoAPI
import MonacoCore
import Observation
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct CabalLeaveLoadTests {
    @Test(.timeLimit(.minutes(1)))
    func theLeaveSlotAsksForMyCabalsBeforeItHasAModel() async throws {
        let transport = StubTransport(.json(.ok, "[]"))
        let model = LeaveCabalModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintSource(),
            cabalID: "cabal-1"
        )
        let window = try Self.window(hosting: CabalLeaveSection(cabalID: "cabal-1") { _, _ in model })
        defer { window.isHidden = true }

        await Self.until { if case .loaded = model.standing { true } else { false } }

        let paths = await transport.sent.map { URLComponents(string: $0.path ?? "")?.path }
        #expect(paths.first == "/v1/me/cabals")
    }

    private static func window(hosting view: CabalLeaveSection) throws -> UIWindow {
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
