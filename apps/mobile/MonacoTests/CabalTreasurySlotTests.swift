import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct CabalTreasurySlotTests {
    @Test func theTreasuryRowIsLive() {
        #expect(CabalTreasurySlot.isLive)
    }

    @Test func solscanLinksToTheTreasuryAccount() {
        let address = "Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf"
        #expect(
            CabalTreasurySlot.solscanURL(for: address)?.absoluteString
                == "https://solscan.io/account/Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf")
    }

    @Test func theTreasuryTabRendersTheTabBodyInPlaceOfTheOldSlots() {
        let names = CabalTabs.sections(for: .treasury).map { String(describing: $0) }
        #expect(names == ["CabalTreasuryTabSlot"])
        #expect(CabalTreasuryTabSlot.isLive)
    }

    @Test func theBotRowStaysHiddenWhileTheBotProposalIsNotLive() {
        #expect(ProposeChooserConfig.isBotProposalLive == false)
        #expect(CabalTreasuryBody.showsBotRow() == false)
        #expect(CabalTreasuryBody.showsBotRow(isLive: true))
    }

    @Test func theOldActivityListRouteIsTheTreasuryHistoryRoute() {
        #expect(CabalActivityListRoute(cabalID: "cabal-1") == CabalTreasuryHistoryRoute(cabalID: "cabal-1"))
    }

    @Test(.timeLimit(.minutes(1)))
    func theBodyListsTheCashPositionsAndTheCardWithoutTheBotRow() async throws {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let rendered = try await Self.render(.sampleACCapital)
        defer { TestWindow.close(rendered.window) }

        let identifiers = AccessibilityTree.identifiers(in: rendered.window)
        for expected in [
            "cabal-holdings-cash", "cabal-holding-NVDA", "cabal-holding-AAPL", "cabal-holding-tSpaceX",
            "cabal-treasury-cash-out", "cabal-treasury-history", "cabal-treasury-footnote",
        ] {
            #expect(identifiers.contains(expected), "\(expected) missing from \(identifiers)")
        }
        #expect(!identifiers.contains("cabal-treasury-bot"), "identifiers: \(identifiers)")
        #expect(!identifiers.contains("cabal-treasury-bot-toggle"), "identifiers: \(identifiers)")
    }

    @Test(.timeLimit(.minutes(1)))
    func cashOutOpensTheCashOutRoute() async throws {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let rendered = try await Self.render(.sampleACCapital)
        defer { TestWindow.close(rendered.window) }

        try Self.activate("cabal-treasury-cash-out", in: rendered.window)

        let route = rendered.opened.routes.last as? CashOutRoute
        #expect(route == CashOutRoute(cabalID: Components.Schemas.CabalPot.sampleACCapital.cabalId))
    }

    @Test(.timeLimit(.minutes(1)))
    func theHistoryRowOpensTreasuryHistory() async throws {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let rendered = try await Self.render(.sampleACCapital)
        defer { TestWindow.close(rendered.window) }

        try Self.activate("cabal-treasury-history", in: rendered.window)

        let route = rendered.opened.routes.last as? CabalTreasuryHistoryRoute
        #expect(route == CabalTreasuryHistoryRoute(cabalID: Components.Schemas.CabalPot.sampleACCapital.cabalId))
    }

    @Test(.timeLimit(.minutes(1)))
    func aPositionOpensItsHolding() async throws {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let rendered = try await Self.render(.sampleACCapital)
        defer { TestWindow.close(rendered.window) }

        try Self.activate("cabal-holding-NVDA", in: rendered.window)

        let route = rendered.opened.routes.last as? CabalHoldingRoute
        #expect(
            route
                == CabalHoldingRoute(cabalID: Components.Schemas.CabalPot.sampleACCapital.cabalId, symbol: "NVDAx"))
    }

    @Test(.timeLimit(.minutes(1)))
    func withoutAShareThereIsNoCashOutRow() async throws {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let rendered = try await Self.render(.noShare)
        defer { TestWindow.close(rendered.window) }

        let identifiers = AccessibilityTree.identifiers(in: rendered.window)
        #expect(!identifiers.contains("cabal-treasury-cash-out"), "identifiers: \(identifiers)")
        #expect(!identifiers.contains("cabal-treasury-footnote"), "identifiers: \(identifiers)")
        #expect(identifiers.contains("cabal-treasury-history"), "identifiers: \(identifiers)")
    }

    @Test(.timeLimit(.minutes(1)))
    func theHeaderTreasuryLineOpensTreasuryHistory() async throws {
        let model = CabalPotModel.preview(.sampleACCapital)
        await model.load()
        var opened = 0
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try ProposalTestWindow.hosting(CabalPotBand(model: model, move: nil) { opened += 1 })
        defer { TestWindow.close(window) }
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains("cabal-pot-value") }

        try Self.activate("cabal-pot-value", in: window)

        #expect(opened == 1)
    }

    final class Opened {
        var routes: [any AppRoute] = []
    }

    private static func render(_ pot: Components.Schemas.CabalPot) async throws -> (
        window: UIWindow, opened: Opened
    ) {
        let model = CabalPotModel.preview(pot)
        await model.load()
        let opened = Opened()
        let window = try ProposalTestWindow.hosting(
            ScrollView {
                CabalTreasuryBody(model: model, cabalID: pot.cabalId, move: nil) { opened.routes.append($0) }
            })
        await ProposalTestWindow.until {
            AccessibilityTree.identifiers(in: window).contains("cabal-treasury-history")
        }
        return (window, opened)
    }

    private static func activate(_ identifier: String, in window: UIWindow) throws {
        let node = try #require(
            AccessibilityTree.nodes(in: window).first {
                $0.responds(to: #selector(getter: UIAccessibilityIdentification.accessibilityIdentifier))
                    && ($0.value(forKey: "accessibilityIdentifier") as? String) == identifier
            }, "no element \(identifier)")
        #expect(node.accessibilityActivate(), "\(identifier) did not activate")
    }
}
