import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct WithdrawScreenTests {
    private static let keypadIDs = [
        "amount-keypad-screen", "amount-keypad-source", "amount-keypad-close", "amount-keypad-amount",
        "amount-keypad-line",
    ]
    private static let presetIDs = ["+$1", "+$10", "+$20", "Max"].map { "amount-keypad-preset-\($0)" }
    private static let keyIDs = (0...9).map { "amount-keypad-\($0)" } + ["amount-keypad-dot", "amount-keypad-delete"]
    private static let fieldIDs = ["withdraw-address-field", "withdraw-paste"]
    private static let networkIDs = ["solana", "base", "arbitrum", "ethereum"].map { "withdraw-network-\($0)" }

    private static func identifiers(of state: WithdrawSampleState, until id: String) async throws -> Set<String> {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try ProposalTestWindow.hosting(WithdrawSampleHarness(state: state).environment(ToastCenter()))
        defer { window.isHidden = true }
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains(id) }
        for _ in 0..<300 { await Task.yield() }
        return Set(AccessibilityTree.identifiers(in: window))
    }

    @Test(.timeLimit(.minutes(1))) func theEmptyStateDrawsTheFieldTheChipsAndTheSlotButNoAction() async throws {
        let ids = try await Self.identifiers(of: .empty, until: "amount-keypad-slot")
        let expected = Self.keypadIDs + Self.presetIDs + Self.keyIDs + Self.fieldIDs + Self.networkIDs
        for id in expected + ["amount-keypad-slot"] {
            #expect(ids.contains(id), "\(id) missing; identifiers: \(ids)")
        }
        #expect(!ids.contains("amount-keypad-action") && !ids.contains("amount-keypad-settled"))
        #expect(!ids.contains("withdraw-network-picker"))
    }

    @Test(.timeLimit(.minutes(1))) func theScanButtonStaysHiddenUntilTheScannerLands() async throws {
        for state in [WithdrawSampleState.empty, .typed, .needsAddress] {
            let ids = try await Self.identifiers(of: state, until: "withdraw-address-field")
            #expect(!ids.contains("withdraw-scan"), "\(state)")
        }
    }

    @Test(.timeLimit(.minutes(1))) func theTypedStateDrawsTheAction() async throws {
        let ids = try await Self.identifiers(of: .typed, until: "amount-keypad-action")
        #expect(ids.contains("amount-keypad-action"), "\(ids)")
        #expect(!ids.contains("amount-keypad-slot") && !ids.contains("amount-keypad-limit-line"))
        #expect(!ids.contains("withdraw-address-problem"))
    }

    @Test(.timeLimit(.minutes(1))) func theInvalidStateDrawsTheLimitLineAndNoAction() async throws {
        let ids = try await Self.identifiers(of: .invalid, until: "amount-keypad-limit-line")
        #expect(ids.contains("amount-keypad-limit-line") && ids.contains("amount-keypad-slot"), "\(ids)")
        #expect(!ids.contains("amount-keypad-action") && !ids.contains("amount-keypad-settled"))
        #expect(Self.presetIDs.allSatisfy { !ids.contains($0) })
        #expect(Self.fieldIDs.allSatisfy { ids.contains($0) } && Self.networkIDs.allSatisfy { ids.contains($0) })
    }

    @Test(.timeLimit(.minutes(1))) func theSettledStateDrawsTheCheck() async throws {
        let ids = try await Self.identifiers(of: .settled, until: "amount-keypad-settled")
        #expect(ids.contains("amount-keypad-settled"), "\(ids)")
        #expect(!ids.contains("amount-keypad-action") && !ids.contains("amount-keypad-slot"))
    }

    @Test(.timeLimit(.minutes(1))) func anAddressProblemShowsUnderTheField() async throws {
        let ids = try await Self.identifiers(of: .invalidAddress, until: "withdraw-address-problem")
        #expect(ids.contains("withdraw-address-problem"), "\(ids)")
        let needs = try await Self.identifiers(of: .needsAddress, until: "amount-keypad-action")
        #expect(needs.contains("amount-keypad-action") && !needs.contains("withdraw-address-problem"), "\(needs)")
    }

    @Test(.timeLimit(.minutes(1))) func anEVMNetworkAndAnUnknownFeeStillDrawTheKeypad() async throws {
        for state in [WithdrawSampleState.evmFee, .feeUnknown, .feeFailed] {
            let ids = try await Self.identifiers(of: state, until: "amount-keypad-line")
            #expect(ids.contains("amount-keypad-line"), "\(state): \(ids)")
        }
    }
}
