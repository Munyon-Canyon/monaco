import Foundation
import MonacoAPI
import MonacoCore
import Observation
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct HomePendingVotesLoadTests {
    @Test(.timeLimit(.minutes(1)))
    func homeAsksForPendingVotesBeforeItHasAnyToShow() async throws {
        let transport = StubTransport(.json(.ok, "[]"))
        let model = PendingVotesModel(
            repository: ProposalsRepository(
                api: APIClient(
                    serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)),
            hints: FakeHintSource()
        )
        let window = try Self.window(hosting: HomePendingVotes(makeModel: { _ in model }))
        defer { window.isHidden = true }

        await Self.until { await !transport.sent.isEmpty }

        let paths = await transport.sent.map { URLComponents(string: $0.path ?? "")?.path }
        #expect(paths.first == "/v1/me/pending-votes")
    }

    private static func window(hosting view: HomePendingVotes) throws -> UIWindow {
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
        for _ in 0..<5_000 {
            if await predicate() { return }
            await Task.yield()
        }
    }
}
