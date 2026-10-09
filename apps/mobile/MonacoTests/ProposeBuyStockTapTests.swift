import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ProposeBuyStockTapTests {
    @Test(.timeLimit(.minutes(1)))
    func oneTapOnASearchResultOpensAmount() async throws {
        let transport = StubTransport(.json(.ok, Self.alphabet))
        let model = MonacoCore.StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintSource(), clock: ImmediateClock())
        try Self.setAutomation(enabled: true)
        defer { try? Self.setAutomation(enabled: false) }
        let (window, navigation) = try Self.window(
            hosting: ProposeBuyStockView(cabalID: "cabal-1", makeModel: { _ in model }))
        defer { window.isHidden = true }
        try #require(await Self.eventually { !model.rows.isEmpty }, "the list never loaded")

        let field = try #require(Self.first(UITextField.self, in: window), "no search field")
        field.becomeFirstResponder()
        field.insertText("GOOGL")
        try #require(await Self.eventually { model.isSearching }, "typing never reached the query")
        try #require(await Self.eventually { model.phase == .loaded && !model.rows.isEmpty }, "no search results")
        let found = await Self.eventually(every: 500) { Self.element("propose-buy-stock-GOOGLx", in: window) != nil }
        let row = try #require(found ? Self.element("propose-buy-stock-GOOGLx", in: window) : nil, "no row")

        let frame = row.accessibilityFrame
        let point = row.accessibilityActivationPoint
        #expect(abs(point.x - frame.midX) < 1 && abs(point.y - frame.midY) < 1, "tap lands at \(point) in \(frame)")
        #expect(row.accessibilityActivate())
        #expect(await Self.eventually { navigation.viewControllers.count > 1 }, "Amount never opened")
    }

    private static let alphabet = #"""
        {"assets":[{"symbol":"GOOGLx","display_name":"Alphabet xStock","issuer":"xstocks","kind":"equity","logo_url":null,"price_micros":347430000,"price_as_of":null,"change_bps":-89,"sparkline_micros":null,"tradable":true,"session":{"state":"open","continuous":false,"holiday":"","early_close":false,"next_state":null,"next_transition":null}}],"next_cursor":null}
        """#

    private static func setAutomation(enabled: Bool) throws {
        let library = try #require(dlopen("/usr/lib/libAccessibility.dylib", RTLD_NOW))
        let symbol = try #require(dlsym(library, "_AXSSetAutomationEnabled"))
        unsafeBitCast(symbol, to: (@convention(c) (Int32) -> Void).self)(enabled ? 1 : 0)
    }

    private static func window(hosting view: ProposeBuyStockView) throws -> (UIWindow, UINavigationController) {
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
        window.layoutIfNeeded()
        let navigation = try #require(first(UINavigationController.self, in: window.rootViewController))
        return (window, navigation)
    }

    private static func first<T>(_: T.Type, in controller: UIViewController?) -> T? {
        guard let controller else { return nil }
        if let match = controller as? T { return match }
        return controller.children.lazy.compactMap { first(T.self, in: $0) }.first
    }

    private static func first<T: UIView>(_: T.Type, in view: UIView) -> T? {
        if let match = view as? T { return match }
        return view.subviews.lazy.compactMap { first(T.self, in: $0) }.first
    }

    private static func element(_ identifier: String, in root: NSObject, depth: Int = 0) -> NSObject? {
        if Self.identifier(of: root) == identifier { return root }
        guard depth < 80 else { return nil }
        var children: [NSObject] = (root as? UIView)?.subviews ?? []
        if let elements = root.accessibilityElements as? [NSObject] {
            children += elements
        } else {
            let count = root.accessibilityElementCount()
            if count != NSNotFound, count > 0 {
                children += (0..<count).compactMap { root.accessibilityElement(at: $0) as? NSObject }
            }
        }
        return children.lazy.compactMap { element(identifier, in: $0, depth: depth + 1) }.first
    }

    private static func identifier(of element: NSObject) -> String? {
        guard element.responds(to: #selector(getter: UIAccessibilityIdentification.accessibilityIdentifier)) else {
            return nil
        }
        return element.value(forKey: "accessibilityIdentifier") as? String
    }

    private static func eventually(every stride: Int = 1, _ predicate: () -> Bool) async -> Bool {
        for _ in 0..<(20_000 / stride) where !predicate() {
            for _ in 0..<stride { await Task.yield() }
        }
        return predicate()
    }
}

private struct ImmediateClock: Clock {
    var now: ContinuousClock.Instant { .now }
    var minimumResolution: Duration { .zero }
    func sleep(until _: ContinuousClock.Instant, tolerance _: Duration?) async throws {}
}
