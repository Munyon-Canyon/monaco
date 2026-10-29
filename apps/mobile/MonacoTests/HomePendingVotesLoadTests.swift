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
        let model = HomePendingVotesModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintSource()
        )
        let window = try Self.window(hosting: HomePendingVotes(makeModel: { _ in model }))
        defer { TestWindow.close(window) }

        await Self.until { await !transport.sent.isEmpty }

        let paths = await transport.sent.map { URLComponents(string: $0.path ?? "")?.path }
        #expect(paths.first == "/v1/me/pending-votes")
    }

    @Test(.timeLimit(.minutes(1)))
    func homeDrawsTwoPendingVotesFromOneRead() async throws {
        let transport = StubTransport(.json(.ok, Self.twoVotes))
        let model = HomePendingVotesModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintSource()
        )
        let window = try Self.window(hosting: HomePendingVotes(makeModel: { _ in model }))
        defer { TestWindow.close(window) }

        await Self.until { model.phase == .loaded }
        for _ in 0..<200 { await Task.yield() }

        let paths = await transport.sent.compactMap { URLComponents(string: $0.path ?? "")?.path }
        #expect(paths == ["/v1/me/pending-votes"])
        #expect(model.cards.count == 2)
        #expect(model.waitingCount == 2)
    }

    private static let twoVotes: String = {
        func vote(_ id: String, _ closes: String) -> String {
            #"{"proposal_id":"\#(id)","cabal_id":"cabal-1","cabal_name":"AC Capital","kind":"buy","rule_change":null,"symbol":"NVDAx","usdc_micros":400000000,"token_amount":null,"asset_name":"NVIDIA","asset_logo_url":null,"proposer":{"user_id":"u","handle":null,"display_name":"Mia","photo_url":null},"tally":{"yes":4,"no":1,"voters":8,"needed":5},"voted_count":5,"reply_count":6,"created_at":"2099-01-01T00:00:00Z","expires_at":"\#(closes)"}"#
        }
        return "[" + vote("p-1", "2099-01-02T00:00:00Z") + "," + vote("p-2", "2099-01-03T00:00:00Z") + "]"
    }()

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
