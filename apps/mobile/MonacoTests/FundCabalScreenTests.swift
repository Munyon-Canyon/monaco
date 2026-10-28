import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct FundCabalScreenTests {
    private static let keypadIDs = [
        "amount-keypad-screen", "amount-keypad-source", "amount-keypad-close", "amount-keypad-amount",
        "amount-keypad-line",
    ]
    private static let presetIDs = ["+$1", "+$10", "+$20", "Max"].map { "amount-keypad-preset-\($0)" }
    private static let keyIDs = (0...9).map { "amount-keypad-\($0)" } + ["amount-keypad-dot", "amount-keypad-delete"]

    private static func identifiers(of state: FundCabalSampleState, until id: String) async throws -> Set<String> {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try ProposalTestWindow.hosting(FundCabalSampleHarness(state: state))
        defer { window.isHidden = true }
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains(id) }
        for _ in 0..<300 { await Task.yield() }
        return Set(AccessibilityTree.identifiers(in: window))
    }

    @Test(.timeLimit(.minutes(1))) func theEmptyStateDrawsTheChipsAndTheSlotButNoAction() async throws {
        let ids = try await Self.identifiers(of: .empty, until: "amount-keypad-slot")
        for id in Self.keypadIDs + Self.presetIDs + Self.keyIDs + ["amount-keypad-slot"] {
            #expect(ids.contains(id), "\(id) missing; identifiers: \(ids)")
        }
        #expect(!ids.contains("amount-keypad-action") && !ids.contains("amount-keypad-settled"))
    }

    @Test(.timeLimit(.minutes(1))) func theTypedStateDrawsTheAction() async throws {
        let ids = try await Self.identifiers(of: .typed, until: "amount-keypad-action")
        #expect(ids.contains("amount-keypad-action"), "\(ids)")
        #expect(!ids.contains("amount-keypad-slot") && !ids.contains("amount-keypad-limit-line"))
    }

    @Test(.timeLimit(.minutes(1))) func theInvalidStateDrawsTheLimitLineAndNoAction() async throws {
        let ids = try await Self.identifiers(of: .invalid, until: "amount-keypad-limit-line")
        #expect(ids.contains("amount-keypad-limit-line") && ids.contains("amount-keypad-slot"), "\(ids)")
        #expect(!ids.contains("amount-keypad-action") && !ids.contains("amount-keypad-settled"))
        #expect(Self.presetIDs.allSatisfy { !ids.contains($0) })
    }

    @Test(.timeLimit(.minutes(1))) func theSettledStateDrawsTheCheck() async throws {
        let ids = try await Self.identifiers(of: .settled, until: "amount-keypad-settled")
        #expect(ids.contains("amount-keypad-settled"), "\(ids)")
        #expect(!ids.contains("amount-keypad-action") && !ids.contains("amount-keypad-slot"))
    }

    @Test(.timeLimit(.minutes(1))) func pausedAndBelowMinimumHaveNoAction() async throws {
        for state in [FundCabalSampleState.paused, .belowMinimum] {
            let ids = try await Self.identifiers(of: state, until: "amount-keypad-line")
            #expect(!ids.contains("amount-keypad-action") && !ids.contains("amount-keypad-settled"), "\(state)")
            #expect(ids.contains("amount-keypad-line"), "\(state): \(ids)")
        }
    }

    @Test(.timeLimit(.minutes(1))) func anUnconfirmedFundKeepsTheActionToRetryOnTheSameKey() async throws {
        let ids = try await Self.identifiers(of: .unconfirmed, until: "amount-keypad-action")
        #expect(ids.contains("amount-keypad-action"), "\(ids)")
        #expect(!ids.contains("amount-keypad-settled"))
    }
}
