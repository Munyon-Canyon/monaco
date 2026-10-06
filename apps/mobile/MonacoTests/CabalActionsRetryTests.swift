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

        tick.value += 1

        await Self.until { model.actions == .member(canPropose: true) }
        await transport.waitForRequests(4)
        #expect(await transport.sent.count == 4)
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
            .environment(\.cabalRetry, CabalRetry(tick: tick.value))
    }
}
