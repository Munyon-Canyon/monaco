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

    @Test(.timeLimit(.minutes(1)))
    func aFailedCabalReadShowsOneErrorRow() async throws {
        let transport = StubTransport(routes: [
            Self.cabalPath: Array(repeating: .failure(URLError(.notConnectedToInternet)), count: 8),
            Self.potPath: Array(repeating: .failure(URLError(.notConnectedToInternet)), count: 8),
            Self.myCabalsPath: Array(repeating: .json(.ok, "[]"), count: 4),
        ])
        let screen = CabalScreen(cabalID: "cabal-1", sections: Self.cabalReadingSections)
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(hosting: screen, over: transport)
        defer { window.isHidden = true }

        await transport.waitForRequest(path: Self.cabalPath)
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains("cabal-failed") }
        for _ in 0..<500 { await Task.yield() }

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.filter { $0 == "cabal-failed" }.count == 1, "identifiers: \(identifiers)")
        let slotErrors = identifiers.filter { $0.hasSuffix("-failed") && $0 != "cabal-failed" }
        #expect(slotErrors.isEmpty, "identifiers: \(identifiers)")
    }

    @Test(
        .timeLimit(.minutes(1)),
        arguments: [
            (role: String?.none, canVote: true, hasMessage: false),
            (role: "member", canVote: false, hasMessage: false),
            (role: "member", canVote: true, hasMessage: true),
        ])
    func onlyVotersAreToldToProposeTheFirstBuy(role: String?, canVote: Bool, hasMessage: Bool) async throws {
        let cabalTransport = StubTransport(routes: [
            Self.cabalPath: [
                .json(.ok, try Self.json(Components.Schemas.Cabal.sample(role: role, canVote: canVote)))
            ]
        ])
        let cabal = CabalModel(
            cabalID: "cabal-1",
            api: APIClient(
                serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: cabalTransport),
            hints: FakeHintSource())
        await cabal.load()
        let emptyPage = #"{"proposals":[],"next_cursor":null}"#
        let proposals = StubTransport(scripted: Array(repeating: .json(.ok, emptyPage), count: 4))
        let list = ProposalListModel(
            cabalID: "cabal-1", filter: .all,
            repository: ProposalsRepository(
                api: APIClient(
                    serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: proposals)),
            hints: EmittingHintSource())
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try ProposalTestWindow.hosting(
            CabalProposals(cabalID: "cabal-1", makeModel: { _, _ in list }).environment(\.cabalModel, cabal))
        defer { window.isHidden = true }

        await ProposalTestWindow.until { AccessibilityTree.labels(in: window).contains("No open votes") }
        for _ in 0..<500 { await Task.yield() }

        let labels = AccessibilityTree.labels(in: window)
        #expect(labels.contains("No open votes"), "labels: \(labels)")
        #expect(labels.contains("Propose the first buy.") == hasMessage, "labels: \(labels)")
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

@MainActor
private enum AccessibilityTree {
    static func setAutomation(enabled: Bool) throws {
        let library = try #require(dlopen("/usr/lib/libAccessibility.dylib", RTLD_NOW))
        let symbol = try #require(dlsym(library, "_AXSSetAutomationEnabled"))
        unsafeBitCast(symbol, to: (@convention(c) (Int32) -> Void).self)(enabled ? 1 : 0)
    }

    static func labels(in root: NSObject) -> [String] {
        walk(root).compactMap { $0.accessibilityLabel }.filter { !$0.isEmpty }
    }

    static func identifiers(in root: NSObject) -> [String] {
        var seen = Set<String>()
        return walk(root).compactMap { node -> String? in
            guard node.responds(to: #selector(getter: UIAccessibilityIdentification.accessibilityIdentifier)),
                let identifier = node.value(forKey: "accessibilityIdentifier") as? String, !identifier.isEmpty
            else { return nil }
            let key = "\(identifier)@\(NSCoder.string(for: node.accessibilityFrame))"
            return seen.insert(key).inserted ? identifier : nil
        }
    }

    private static func walk(_ node: NSObject, depth: Int = 0) -> [NSObject] {
        guard depth < 80 else { return [] }
        var children: [NSObject] = (node as? UIView)?.subviews ?? []
        if let elements = node.accessibilityElements as? [NSObject] {
            children += elements
        } else {
            let count = node.accessibilityElementCount()
            if count != NSNotFound, count > 0 {
                children += (0..<count).compactMap { node.accessibilityElement(at: $0) as? NSObject }
            }
        }
        return [node] + children.flatMap { walk($0, depth: depth + 1) }
    }
}
