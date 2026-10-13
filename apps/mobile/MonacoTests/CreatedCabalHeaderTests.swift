import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct CreatedCabalHeaderTests {
    @Test(.timeLimit(.minutes(1)))
    func aCreatedCabalOpensWithItsNameBelowTheNavigationBar() async throws {
        let cabal = Components.Schemas.Cabal.sample(role: "creator")
        let body = try Self.json(cabal)
        let transport = StubTransport(routes: [
            "/v1/cabals": [.json(.created, body)],
            "/v1/cabals/\(cabal.id)": Array(repeating: .json(.ok, body), count: 4),
            "/v1/cabals/\(cabal.id)/pot": Array(
                repeating: .json(.ok, try Self.json(Components.Schemas.CabalPot.sampleZero)), count: 4),
        ])
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let (window, navigator) = try Self.window(over: transport)
        defer { window.isHidden = true }

        let field = try #require(await Self.eventually { Self.first(UITextField.self, in: window) }, "no name field")
        field.becomeFirstResponder()
        field.insertText(cabal.name)
        let submit = try #require(
            await Self.eventually { Self.element("create-group-submit", in: window, enabled: true) },
            "Create cabal never enabled")
        #expect(submit.accessibilityActivate())

        let header = try #require(
            await Self.eventually { Self.element("cabal-header-name", in: window) }, "the cabal never opened")
        #expect(navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: cabal.id))])
        var report = ""
        let visible = await Self.eventually { () -> Bool? in
            guard let bar = Self.first(UINavigationBar.self, in: window) else { return nil }
            let barBottom = bar.convert(bar.bounds, to: nil).maxY
            report = "header at \(header.accessibilityFrame), bar ends at \(barBottom)"
            return header.accessibilityFrame.minY >= barBottom - 0.5
                && header.accessibilityFrame.maxY <= window.bounds.maxY ? true : nil
        }
        #expect(visible == true, "\(report)")
    }

    private static func window(over transport: StubTransport) throws -> (UIWindow, AppNavigator) {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let environment = AppEnvironment(
            auth: auth, api: api, hints: FakeHintSource(), isAuthenticated: { true }, endAuthSession: {})
        let navigator = environment.navigator
        navigator.open(CreateCabalRoute(), in: .cabals)
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: CabalsStack(navigator: navigator)
                .environment(environment)
                .environment(ToastCenter())
        )
        window.makeKeyAndVisible()
        return (window, navigator)
    }

    private static func json(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private static func element(_ identifier: String, in root: NSObject, enabled: Bool = false) -> NSObject? {
        if idOf(root) == identifier, !enabled || !root.accessibilityTraits.contains(.notEnabled) {
            return root
        }
        var children: [NSObject] = (root as? UIView)?.subviews ?? []
        if let elements = root.accessibilityElements as? [NSObject] {
            children += elements
        } else {
            let count = root.accessibilityElementCount()
            if count != NSNotFound, count > 0 {
                children += (0..<count).compactMap { root.accessibilityElement(at: $0) as? NSObject }
            }
        }
        for child in children {
            if let found = element(identifier, in: child, enabled: enabled) { return found }
        }
        return nil
    }

    private static func idOf(_ element: NSObject) -> String? {
        guard element.responds(to: #selector(getter: UIAccessibilityIdentification.accessibilityIdentifier)) else {
            return nil
        }
        return element.value(forKey: "accessibilityIdentifier") as? String
    }

    private static func first<T: UIView>(_: T.Type, in view: UIView) -> T? {
        if let match = view as? T { return match }
        for subview in view.subviews {
            if let found = first(T.self, in: subview) { return found }
        }
        return nil
    }

    private static func eventually<T>(_ value: () -> T?) async -> T? {
        for _ in 0..<20_000 {
            if let found = value() { return found }
            await nextRunLoopTurn()
        }
        return value()
    }
}

private struct CabalsStack: View {
    @Bindable var navigator: AppNavigator

    var body: some View {
        NavigationStack(path: $navigator.cabalsPath) {
            Color.clear.tabRouteDestinations()
        }
    }
}

@MainActor
private func nextRunLoopTurn() async {
    await withCheckedContinuation { continuation in
        RunLoop.main.perform { continuation.resume() }
    }
}
