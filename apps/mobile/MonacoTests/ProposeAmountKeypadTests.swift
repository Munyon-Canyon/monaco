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
    func theThesisFieldIsAlwaysThereAndTheAddAReasonButtonIsGone() async throws {
        try Self.setAutomation(enabled: true)
        defer { try? Self.setAutomation(enabled: false) }
        let (window, _) = try Self.window()
        defer { window.isHidden = true }

        #expect(await Self.eventually(within: 10) { Self.element("propose-amount-reason", in: window) != nil })
        #expect(Self.element("propose-amount-add-reason", in: window) == nil)
        #expect(Self.element("amount-keypad", in: window) == nil)
    }

    @Test(.timeLimit(.minutes(1)))
    func theAmountFigureStartsClearOfTheBarAtTheLargestTextSize() async throws {
        try Self.setAutomation(enabled: true)
        defer { try? Self.setAutomation(enabled: false) }
        let (window, navigation) = try Self.window(textSize: .accessibility5, size: CGSize(width: 440, height: 956))
        defer { window.isHidden = true }
        func barBottom() -> CGFloat { navigation.navigationBar.convert(navigation.navigationBar.bounds, to: nil).maxY }

        let layoutSlack: CGFloat = 0.5
        let reached = await Self.eventually(within: 10) {
            guard let figure = Self.element("amount-entry-field", in: window) else { return false }
            return figure.accessibilityFrame.minY >= barBottom() - layoutSlack
        }
        #expect(reached, "the figure starts clear of the bar")
    }

    @Test(.timeLimit(.minutes(1)))
    func theHelperSaysItIsFindingThePriceUntilThePreviewReturns() async throws {
        try Self.setAutomation(enabled: true)
        defer { try? Self.setAutomation(enabled: false) }
        let service = HeldPreviewService()
        let (window, _) = try Self.window(service: service)
        defer { window.isHidden = true }
        func helper() -> String? { Self.element("amount-entry-helper", in: window)?.accessibilityLabel }
        func spinning() -> Bool { Self.element("amount-entry-spinner", in: window) != nil }
        #expect(await Self.eventually(within: 10) { helper() == "The cabal can spend $500.00" })

        try await Self.typeOne(in: window)

        #expect(await Self.eventually(within: 10) { helper() == "Finding the best price…" && spinning() })
        while await !service.isAsked() { await Task.yield() }
        #expect(helper() == "Finding the best price…")
        await service.answer()
        #expect(await Self.eventually(within: 10) { helper() == "The cabal can spend $500.00" && !spinning() })
    }

    @Test(.timeLimit(.minutes(1)))
    func theTitleNamesTheTrade() async throws {
        let trades: [(MonacoCore.ProposeTrade?, String)] = [
            (nil, "Buy Alphabet"), (.sell(Self.holding), "Sell Alphabet"),
        ]
        for (trade, title) in trades {
            let (window, navigation) = try Self.window(trade: trade)
            defer { window.isHidden = true }
            _ = await Self.eventually { navigation.navigationBar.topItem?.title == title }
            #expect(navigation.navigationBar.topItem?.title == title)
        }
    }

    actor HeldPreviewService: MonacoCore.ProposeService {
        private var held: CheckedContinuation<Void, Never>?
        private(set) var networks: [Components.Schemas.Network?] = []
        private(set) var answered = false

        func isAsked() -> Bool { held != nil }

        func answer() {
            answered = true
            held?.resume()
            held = nil
        }

        func preview(cabalID _: String, draft: ProposalDraft, network: Components.Schemas.Network?) async throws
            -> ProposePreview
        {
            if draft.amount > 0 {
                networks.append(network)
                await withCheckedContinuation { held = $0 }
            }
            return ProposePreview(
                .init(
                    quoteOutAmount: nil, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 500_000_000,
                    spendableCashMicros: 500_000_000, maxBuyMicros: 499_000_000))
        }
        func propose(
            cabalID _: String, draft _: ProposalDraft, network _: Components.Schemas.Network?,
            submission _: IdempotentSubmission
        ) async throws -> String {
            throw CancellationError()
        }
        func withdraw(proposalID _: String, submission _: IdempotentSubmission) async throws {
            throw CancellationError()
        }
    }

    struct UnansweredProposeService: MonacoCore.ProposeService {
        func preview(cabalID _: String, draft _: ProposalDraft, network _: Components.Schemas.Network?) async throws
            -> ProposePreview
        {
            throw CancellationError()
        }
        func propose(
            cabalID _: String, draft _: ProposalDraft, network _: Components.Schemas.Network?,
            submission _: IdempotentSubmission
        ) async throws -> String {
            throw CancellationError()
        }
        func withdraw(proposalID _: String, submission _: IdempotentSubmission) async throws {
            throw CancellationError()
        }
    }

    static func typeOne(in window: UIWindow) async throws {
        #expect(await eventually(within: 10) { firstResponder(in: window) is UITextField })
        let field = try #require(firstResponder(in: window) as? UITextField)
        field.insertText("1")
    }

    static func setAutomation(enabled: Bool) throws {
        try AccessibilityTree.setAutomation(enabled: enabled)
    }

    private static var holding: ProposeHolding {
        ProposeHolding(Components.Schemas.CabalPot.sampleInvested.holdings[0])
    }

    static func window(
        textSize: DynamicTypeSize = .large, trade: MonacoCore.ProposeTrade? = nil, size: CGSize? = nil,
        service: any MonacoCore.ProposeService = UnansweredProposeService(),
        bestChains: MonacoCore.AssetBestChainStore? = nil
    ) throws -> (UIWindow, UINavigationController) {
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
            service: service, cabalID: "cabal-1", stock: ProposeStock(symbol: "GOOGLx"),
            trade: trade, bestChains: bestChains)
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack { screen }
                .environment(environment)
                .environment(ToastCenter())
                .dynamicTypeSize(textSize)
        )
        if let size { window.frame = CGRect(origin: .zero, size: size) }
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

    static func firstResponder(in view: UIView) -> UIView? {
        if view.isFirstResponder { return view }
        return view.subviews.lazy.compactMap { firstResponder(in: $0) }.first
    }

    static func element(_ identifier: String, in root: NSObject, depth: Int = 0) -> NSObject? {
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

    static func identifier(of element: NSObject) -> String? {
        guard element.responds(to: #selector(getter: UIAccessibilityIdentification.accessibilityIdentifier)) else {
            return nil
        }
        return element.value(forKey: "accessibilityIdentifier") as? String
    }

    static func eventually(within seconds: Double = 3, _ predicate: () -> Bool) async -> Bool {
        let deadline = Date().addingTimeInterval(seconds)
        while !predicate() {
            if Date() > deadline { return false }
            await withCheckedContinuation { continuation in
                RunLoop.main.perform { continuation.resume() }
            }
        }
        return true
    }

    private static func until(_ predicate: () -> Bool) async {
        while !predicate() {
            await withCheckedContinuation { continuation in
                RunLoop.main.perform { continuation.resume() }
            }
        }
    }
}
