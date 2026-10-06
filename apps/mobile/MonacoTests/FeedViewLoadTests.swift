import Foundation
import MonacoAPI
import MonacoCore
import Observation
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct FeedViewLoadTests {
    @Test(.timeLimit(.minutes(1)))
    func theFeedTabAsksForAPageAndShowsIt() async throws {
        let transport = StubTransport(.json(.ok, try Self.page(Components.Schemas.FeedItem.samples)))
        let model = FeedModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            viewerID: nil,
            hints: FakeHintSource(),
            clock: ContinuousClock()
        )
        let window = try Self.window(hosting: FeedView(makeModel: { _ in model }))
        defer { window.isHidden = true }

        await Self.until { model.phase == .loaded }

        let paths = await transport.sent.map { URLComponents(string: $0.path ?? "")?.path }
        #expect(paths.first == "/v1/feed")
        #expect(model.items.count == Components.Schemas.FeedItem.samples.count)
    }

    private static func window(hosting view: FeedView) throws -> UIWindow {
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

    private static func page(_ items: [Components.Schemas.FeedItem]) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let page = Components.Schemas.FeedPage(items: items, nextCursor: nil)
        return String(decoding: try encoder.encode(page), as: UTF8.self)
    }
}
