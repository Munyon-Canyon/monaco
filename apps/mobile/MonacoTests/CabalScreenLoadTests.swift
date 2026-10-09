import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct CabalScreenLoadTests {
    private static let cabalPath = "/v1/cabals/cabal-1"
    private static let potPath = "/v1/cabals/cabal-1/pot"
    private static let myCabalsPath = "/v1/me/cabals"

    @Test(.timeLimit(.minutes(1)))
    func openingACabalReadsItAndItsPotOnce() async throws {
        let transport = StubTransport(routes: [
            Self.cabalPath: [.json(.ok, try Self.json(Components.Schemas.Cabal.sample(role: "member")))],
            Self.potPath: [.json(.ok, try Self.json(Components.Schemas.CabalPot.sampleZero))],
            Self.myCabalsPath: Array(repeating: .json(.ok, "[]"), count: 4),
        ])
        let screen = CabalScreen(cabalID: "cabal-1", sections: Self.cabalReadingSections)
        let window = try Self.window(hosting: screen, over: transport)
        defer { window.isHidden = true }

        await transport.waitForRequest(path: Self.cabalPath)
        await transport.waitForRequest(path: Self.potPath)
        for _ in 0..<2_000 { await Task.yield() }

        let paths = await transport.sent.map { URLComponents(string: $0.path ?? "")?.path }
        #expect(paths.filter { $0 == Self.cabalPath }.count == 1, "requests sent: \(paths)")
        #expect(paths.filter { $0 == Self.potPath }.count == 1, "requests sent: \(paths)")
    }

    private static let cabalReadingSections: [any CabalSection.Type] = [
        CabalHeaderSlot.self,
        CabalPotSlot.self,
        CabalSliceSlot.self,
        CabalJoinSlot.self,
        CabalActionsSlot.self,
        CabalHoldingsSlot.self,
    ]

    private static func json(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private static func window(hosting view: CabalScreen, over transport: StubTransport) throws -> UIWindow {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let environment = AppEnvironment(
            auth: auth, api: api, hints: FakeHintSource(), isAuthenticated: { true }, endAuthSession: {})
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack { view }
                .environment(environment)
                .environment(ToastCenter())
        )
        window.makeKeyAndVisible()
        return window
    }
}
