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
    private static let historyPath = "/v1/cabals/cabal-1/value-history"
    private static let proposalsPath = "/v1/cabals/cabal-1/proposals"

    @Test(.timeLimit(.minutes(1)))
    func openingACabalReadsItAndItsPotOnce() async throws {
        let transport = StubTransport(routes: [
            Self.cabalPath: [.json(.ok, try Self.json(Components.Schemas.Cabal.sample(role: "member")))],
            Self.potPath: [.json(.ok, try Self.json(Components.Schemas.CabalPot.sampleZero))],
            Self.myCabalsPath: Array(repeating: .json(.ok, "[]"), count: 4),
        ])
        let screen = CabalScreen(cabalID: "cabal-1", sections: Self.cabalReadingSections)
        let window = try Self.window(hosting: screen, over: transport)
        defer { TestWindow.close(window) }

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
        defer { TestWindow.close(window) }

        await transport.waitForRequest(path: Self.cabalPath)
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains("cabal-failed") }
        for _ in 0..<500 { await Task.yield() }

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.filter { $0 == "cabal-failed" }.count == 1, "identifiers: \(identifiers)")
        let slotErrors = identifiers.filter { $0.hasSuffix("-failed") && $0 != "cabal-failed" }
        #expect(slotErrors.isEmpty, "identifiers: \(identifiers)")
    }

    @Test(.timeLimit(.minutes(1)))
    func theHeaderReadsTheCabalThePotTheMonthAndTheOpenProposalsOnce() async throws {
        let transport = try Self.transport(
            cabal: .sample(role: "member"), pot: .sampleInvested, proposals: [.sample(status: .open)])
        let screen = CabalScreen(cabalID: "cabal-1", sections: Self.headerSections)
        let window = try Self.window(hosting: screen, over: transport)
        defer { TestWindow.close(window) }

        await transport.waitForRequest(path: Self.historyPath)
        await transport.waitForRequest(path: Self.proposalsPath)
        for _ in 0..<2_000 { await Task.yield() }

        let sent = await transport.sent
        let paths = sent.map { URLComponents(string: $0.path ?? "")?.path }
        for path in [Self.cabalPath, Self.potPath, Self.historyPath, Self.proposalsPath] {
            #expect(paths.filter { $0 == path }.count == 1, "\(path) requests sent: \(paths)")
        }
        let history = try #require(sent.first { $0.path?.hasPrefix(Self.historyPath) == true })
        #expect(history.path?.contains("range=1M") == true, "history request: \(history.path ?? "")")
    }

    @Test(.timeLimit(.minutes(1)))
    func openProposalsShowOnACabalWithCashAndNoPositions() async throws {
        let transport = try Self.transport(
            cabal: .sample(role: "member"), pot: .sampleCashOnly, proposals: [.sample(status: .open)])
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(
            hosting: CabalScreen(cabalID: "cabal-1", sections: Self.headerSections), over: transport)
        defer { TestWindow.close(window) }

        await ProposalTestWindow.until {
            AccessibilityTree.identifiers(in: window).contains("cabal-open-proposal-proposal-1")
        }

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.contains("cabal-open-proposal-proposal-1"), "identifiers: \(identifiers)")
    }

    @Test(.timeLimit(.minutes(1)), arguments: [(canPropose: true, hasPropose: true), (false, false)])
    func proposeShowsOnlyWhenTheMemberMayPropose(canPropose: Bool, hasPropose: Bool) async throws {
        var cabal = Components.Schemas.Cabal.sample(role: "member")
        cabal.me?.canPropose = canPropose
        let transport = try Self.transport(cabal: cabal, pot: .sampleInvested, proposals: [])
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(
            hosting: CabalScreen(cabalID: "cabal-1", sections: Self.headerSections), over: transport)
        defer { TestWindow.close(window) }

        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains("cabal-action-fund") }
        for _ in 0..<500 { await Task.yield() }

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.contains("cabal-action-propose") == hasPropose, "identifiers: \(identifiers)")
        #expect(identifiers.contains("cabal-action-fund"), "identifiers: \(identifiers)")
    }

    private static let cabalReadingSections: [any CabalSection.Type] = [
        CabalHeaderSlot.self,
        CabalPotSlot.self,
        CabalSliceSlot.self,
        CabalJoinSlot.self,
        CabalActionsSlot.self,
        CabalHoldingsSlot.self,
    ]

    private static let headerSections: [any CabalSection.Type] = [
        CabalHeaderSlot.self,
        CabalPotSlot.self,
        CabalSliceSlot.self,
        CabalActionsSlot.self,
        CabalProposalsSlot.self,
    ]

    private static func transport(
        cabal: Components.Schemas.Cabal, pot: Components.Schemas.CabalPot, proposals: [Components.Schemas.Proposal]
    ) throws -> StubTransport {
        let page = Components.Schemas.ProposalList(proposals: proposals, nextCursor: nil)
        return StubTransport(routes: [
            cabalPath: Array(repeating: .json(.ok, try json(cabal)), count: 8),
            potPath: Array(repeating: .json(.ok, try json(pot)), count: 8),
            "\(historyPath)?range=1M": Array(
                repeating: .json(.ok, try json(Components.Schemas.CabalValueHistory.sampleCurve())), count: 8),
            "\(proposalsPath)?filter=all": Array(repeating: .json(.ok, try json(page)), count: 8),
            myCabalsPath: Array(repeating: .json(.ok, "[]"), count: 8),
        ])
    }

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
enum AccessibilityTree {
    private static var holders = 0

    static func setAutomation(enabled: Bool) throws {
        if enabled {
            holders += 1
            guard holders == 1 else { return }
        } else {
            guard holders > 0 else { return }
            holders -= 1
            guard holders == 0 else { return }
        }
        let library = try #require(dlopen("/usr/lib/libAccessibility.dylib", RTLD_NOW))
        let symbol = try #require(dlsym(library, "_AXSSetAutomationEnabled"))
        unsafeBitCast(symbol, to: (@convention(c) (Int32) -> Void).self)(enabled ? 1 : 0)
    }

    static func labels(in root: NSObject) -> [String] {
        walk(root).compactMap { $0.accessibilityLabel }.filter { !$0.isEmpty }
    }

    static func nodes(in root: NSObject) -> [NSObject] {
        walk(root)
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
        let children: [NSObject]
        let count = node.accessibilityElementCount()
        if let elements = node.accessibilityElements as? [NSObject] {
            children = elements
        } else if count != NSNotFound, count > 0 {
            children = (0..<count).compactMap { node.accessibilityElement(at: $0) as? NSObject }
        } else {
            children = (node as? UIView)?.subviews ?? []
        }
        return [node] + children.flatMap { walk($0, depth: depth + 1) }
    }
}
