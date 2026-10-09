import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ProposeAmountKeypadTests {
    @Test(.timeLimit(.minutes(1)))
    func theKeypadComesBackWhenTheReasonFieldLetsGo() async throws {
        try Self.setAutomation(enabled: true)
        defer { try? Self.setAutomation(enabled: false) }
        let (window, _) = try Self.window()
        defer { window.isHidden = true }
        await Self.until {
            Self.element("amount-keypad", in: window) != nil
                && Self.element("propose-amount-add-reason", in: window) != nil
        }

        let addReason = try #require(Self.element("propose-amount-add-reason", in: window))
        #expect(addReason.accessibilityActivate())
        await Self.until { Self.firstResponder(in: window) != nil }
        let reason = try #require(Self.firstResponder(in: window))
        await Self.until { Self.element("amount-keypad", in: window) == nil }

        reason.resignFirstResponder()

        await Self.until { Self.element("amount-keypad", in: window) != nil }
        let five = try #require(Self.element("amount-keypad-5", in: window))
        #expect(five.accessibilityActivate())
        await Self.until { Self.element("amount-entry-field", in: window)?.accessibilityValue == "$5" }
    }

    @Test(.timeLimit(.minutes(1)))
    func theAmountFigureComesBackIntoViewWhenTheMemberTypes() async throws {
        try Self.setAutomation(enabled: true)
        defer { try? Self.setAutomation(enabled: false) }
        let (window, navigation) = try Self.window(textSize: .accessibility5)
        defer { window.isHidden = true }
        let barBottom = navigation.navigationBar.convert(navigation.navigationBar.bounds, to: nil).maxY

        func figureIsClear() -> Bool {
            guard let figure = Self.element("amount-entry-field", in: window),
                let keypad = Self.element("amount-keypad", in: window)
            else { return false }
            let frame = figure.accessibilityFrame
            return frame.minY >= barBottom && frame.maxY <= keypad.accessibilityFrame.minY
        }

        await Self.until { figureIsClear() }
        let scroll = try #require(Self.first(UIScrollView.self, in: window))
        scroll.setContentOffset(CGPoint(x: 0, y: -scroll.adjustedContentInset.top), animated: false)
        await Self.until { !figureIsClear() }
        var typed = ""
        for digit in [1, 2, 3] {
            let key = try #require(Self.element("amount-keypad-\(digit)", in: window))
            #expect(key.accessibilityActivate())
            typed += String(digit)
            await Self.until { Self.element("amount-entry-field", in: window)?.accessibilityValue == "$\(typed)" }
            await Self.until { figureIsClear() }
        }
    }

    private struct UnansweredProposeService: MonacoCore.ProposeService {
        func preview(cabalID _: String, draft _: ProposalDraft) async throws -> ProposePreview {
            throw CancellationError()
        }
        func potValue(cabalID _: String) async throws -> Int64 { throw CancellationError() }
        func propose(cabalID _: String, draft _: ProposalDraft, submission _: IdempotentSubmission) async throws
            -> String
        {
            throw CancellationError()
        }
        func withdraw(proposalID _: String, submission _: IdempotentSubmission) async throws {
            throw CancellationError()
        }
    }

    private static func setAutomation(enabled: Bool) throws {
        let library = try #require(dlopen("/usr/lib/libAccessibility.dylib", RTLD_NOW))
        let symbol = try #require(dlsym(library, "_AXSSetAutomationEnabled"))
        unsafeBitCast(symbol, to: (@convention(c) (Int32) -> Void).self)(enabled ? 1 : 0)
    }

    private static func window(textSize: DynamicTypeSize = .large) throws -> (UIWindow, UINavigationController) {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: StubTransport(.hang))
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(), api: api, hints: FakeHintSource(),
            isAuthenticated: { true }, endAuthSession: {})
        let screen = ProposeAmountScreen(
            service: UnansweredProposeService(), cabalID: "cabal-1", stock: ProposeStock(symbol: "GOOGLx"))
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack { screen }
                .environment(environment)
                .environment(ToastCenter())
                .dynamicTypeSize(textSize)
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

    private static func firstResponder(in view: UIView) -> UIView? {
        if view.isFirstResponder { return view }
        return view.subviews.lazy.compactMap { firstResponder(in: $0) }.first
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

    private static func until(_ predicate: () -> Bool) async {
        while !predicate() {
            await withCheckedContinuation { continuation in
                RunLoop.main.perform { continuation.resume() }
            }
        }
    }
}
