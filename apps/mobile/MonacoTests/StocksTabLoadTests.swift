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
        defer { TestWindow.close(window) }

        await Self.until { model.preIpo.phase != .idle && model.preIpo.phase != .loading }

        let paths = await transport.sent.map { URLComponents(string: $0.path ?? "")?.path }
        #expect(paths.first == "/v1/assets")
        #expect(model.isMarkets)
    }

    @Test(.timeLimit(.minutes(1)))
    func theStocksTabShowsStocksAndPreIpoTogether() async throws {
        var apple = Components.Schemas.AssetSummary.googl
        apple.symbol = "AAPLx"
        apple.displayName = "Apple xStock"
        let transport = StubTransport(routes: [
            "/v1/assets?filter=popular&limit=20": [.json(.ok, try Self.json(.init(assets: [apple], nextCursor: nil)))],
            "/v1/assets?filter=pre_ipo&limit=20": [
                .json(.ok, try Self.json(.init(assets: [.spaceX], nextCursor: nil)))
            ],
            "/v1/assets?filter=movers&limit=10": [.json(.ok, try Self.json(.init(assets: [apple], nextCursor: nil)))],
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = MonacoCore.StocksTabModel(api: api, hints: FakeHintSource(), clock: ContinuousClock())
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(hosting: StocksTabView(makeModel: { _ in model }))
        defer { TestWindow.close(window) }

        await ProposalTestWindow.until {
            let identifiers = AccessibilityTree.identifiers(in: window)
            return identifiers.contains("markets-stocks-row-AAPLx")
                && identifiers.contains("markets-preipo-row-tSpaceX")
        }

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.contains("markets-stocks"), "identifiers: \(identifiers)")
        #expect(identifiers.contains("markets-preipo"), "identifiers: \(identifiers)")
    }

    private static func json(_ list: Components.Schemas.AssetList) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(list), as: UTF8.self)
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
