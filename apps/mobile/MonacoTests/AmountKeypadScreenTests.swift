import SwiftUI
import Testing
import UIKit

@testable import Monaco

private struct KeypadHarness: View {
    @State var text: String
    let state: AmountKeypadState
    var withInfo = true

    var body: some View {
        AmountKeypadScreen(
            amountText: $text, state: state,
            line: AmountKeypadLine(text: "From your cash · your share 35% → 36%", info: withInfo ? {} : nil),
            max: 500,
            action: AmountKeypadAction(title: "Fund $250", settledTitle: "$250 to AC Capital", perform: {}),
            onClose: {},
            source: { Text("AC Capital") },
            header: { EmptyView() })
    }
}

@MainActor
struct AmountKeypadScreenTests {
    private static let limit = AmountKeypadState.invalid(
        limitLine: "$500.00 available · $150.00 over", slot: "Up to $500.00")
    private static let settled = AmountKeypadState.settled(line: "Added just now · your share is now 36%")
    private static let presetIDs = ["+$1", "+$10", "+$20", "Max"].map { "amount-keypad-preset-\($0)" }
    private static let keyIDs = (0...9).map { "amount-keypad-\($0)" } + ["amount-keypad-dot", "amount-keypad-delete"]
    private static let chromeIDs = [
        "amount-keypad-screen", "amount-keypad-source", "amount-keypad-close", "amount-keypad-amount",
        "amount-keypad-line",
    ]

    private static func nodes(in root: NSObject, depth: Int = 0) -> [NSObject] {
        guard depth < 80 else { return [] }
        var children: [NSObject] = (root as? UIView)?.subviews ?? []
        if let elements = root.accessibilityElements as? [NSObject] {
            children += elements
        } else {
            let count = root.accessibilityElementCount()
            if count != NSNotFound, count > 0 {
                children += (0..<count).compactMap { root.accessibilityElement(at: $0) as? NSObject }
            }
        }
        return [root] + children.flatMap { nodes(in: $0, depth: depth + 1) }
    }

    private static func identifier(_ node: NSObject) -> String? {
        guard node.responds(to: #selector(getter: UIAccessibilityIdentification.accessibilityIdentifier)) else {
            return nil
        }
        let value = node.value(forKey: "accessibilityIdentifier") as? String
        return value?.isEmpty == false ? value : nil
    }

    private static func shown(
        _ state: AmountKeypadState, text: String, size: DynamicTypeSize = .large,
        run: ([NSObject], UIWindow) throws -> Void
    ) async throws {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try ProposalTestWindow.hosting(
            KeypadHarness(text: text, state: state).environment(\.dynamicTypeSize, size))
        defer { window.isHidden = true }
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains("amount-keypad-amount") }
        for _ in 0..<300 { await Task.yield() }
        try run(nodes(in: window), window)
    }

    private static func ids(_ nodes: [NSObject]) -> Set<String> {
        Set(nodes.compactMap(identifier))
    }

    @Test func emptyShowsTheSlotAndNoButton() async throws {
        try await Self.shown(.empty, text: "") { nodes, _ in
            let ids = Self.ids(nodes)
            #expect(ids.contains("amount-keypad-slot"), "\(ids)")
            #expect(!ids.contains("amount-keypad-action") && !ids.contains("amount-keypad-settled"))
            #expect(!ids.contains("amount-keypad-limit-line"))
            #expect(Set(Self.presetIDs).isSubset(of: ids), "\(ids)")
        }
    }

    @Test func typedShowsTheActionAndNoSlot() async throws {
        try await Self.shown(.typed, text: "250") { nodes, _ in
            let ids = Self.ids(nodes)
            #expect(ids.contains("amount-keypad-action"), "\(ids)")
            #expect(!ids.contains("amount-keypad-slot") && !ids.contains("amount-keypad-settled"))
            #expect(Set(Self.presetIDs).isSubset(of: ids))
        }
    }

    @Test func invalidReplacesTheChipsWithTheLimitLineAndHasNoButton() async throws {
        try await Self.shown(Self.limit, text: "650") { nodes, _ in
            let ids = Self.ids(nodes)
            #expect(ids.contains("amount-keypad-limit-line"), "\(ids)")
            #expect(ids.contains("amount-keypad-slot"))
            #expect(!ids.contains("amount-keypad-action") && !ids.contains("amount-keypad-settled"))
            #expect(Self.presetIDs.allSatisfy { !ids.contains($0) }, "\(ids)")
        }
    }

    @Test func settledShowsTheCheckButtonAndDisablesKeysAndChips() async throws {
        try await Self.shown(Self.settled, text: "250") { nodes, _ in
            let ids = Self.ids(nodes)
            #expect(ids.contains("amount-keypad-settled"), "\(ids)")
            #expect(!ids.contains("amount-keypad-action") && !ids.contains("amount-keypad-slot"))
            let disabled = Set(
                nodes.filter { $0.accessibilityTraits.contains(.notEnabled) }.compactMap(Self.identifier))
            for id in Self.keyIDs + Self.presetIDs {
                #expect(disabled.contains(id), "\(id) is still enabled; disabled: \(disabled)")
            }
        }
    }

    @Test func everyIdentifierInTheScopeListIsPresentInTheStateThatDrawsIt() async throws {
        try await Self.shown(.typed, text: "250") { nodes, _ in
            let ids = Self.ids(nodes)
            for id in Self.chromeIDs + Self.presetIDs + Self.keyIDs + ["amount-keypad-action"] {
                #expect(ids.contains(id), "\(id) missing; identifiers: \(ids)")
            }
        }
    }

    @Test func maxFillsTheMaximumAndChipsAdd() {
        #expect(AmountPreset.fraction(1, label: "Max").applied(to: "12", max: 499.5) == "499.5")
        #expect(AmountPreset.fraction(1, label: "Max").applied(to: "12", max: nil) == nil)
        #expect(AmountPreset.add(10).applied(to: "12.5", max: 5) == "22.5")
        #expect(AmountPreset.add(1).label == "+$1")
        #expect(AmountPreset.add(20).label == "+$20")
    }

    @Test func theAmountIsSecondaryWhenEmptyAndPrimaryWhenTyped() {
        #expect(AmountKeypadColors.amount(for: "") == MonacoTheme.secondaryText)
        #expect(AmountKeypadColors.amount(for: "0") == MonacoTheme.primaryText)
        #expect(AmountKeypadColors.amount(for: "250") == MonacoTheme.primaryText)
    }

    @Test(.timeLimit(.minutes(1))) func keysStayTouchableAndTheLayoutFitsAtAccessibility3() async throws {
        try await Self.shown(.typed, text: "250", size: .accessibility3) { nodes, window in
            let bounds = window.screen.bounds
            var frames: [String: CGRect] = [:]
            for node in nodes {
                if let id = Self.identifier(node), frames[id] == nil { frames[id] = node.accessibilityFrame }
            }
            for id in Self.keyIDs {
                let frame = try #require(frames[id], "\(id) missing")
                #expect(frame.height >= 44 && frame.width >= 44, "\(id) is \(frame)")
            }
            for id in Self.presetIDs + ["amount-keypad-action", "amount-keypad-close", "amount-keypad-amount"] {
                let frame = try #require(frames[id], "\(id) missing")
                #expect(frame.minY >= 0 && frame.maxY <= bounds.height, "\(id) is clipped: \(frame)")
            }
        }
    }
}
