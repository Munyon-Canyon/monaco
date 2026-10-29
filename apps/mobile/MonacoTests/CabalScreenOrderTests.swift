import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct CabalScreenOrderTests {
    private static let cabalPath = "/v1/cabals/cabal-1"
    private static let potPath = "/v1/cabals/cabal-1/pot"
    private static let myCabalsPath = "/v1/me/cabals"
    private static let actionIDs = ["cabal-action-fund", "cabal-action-propose"]

    enum Variant: String, CaseIterable, Sendable, CustomTestStringConvertible {
        case member
        case noShare = "no share"
        case cashOnly = "cash only"
        case closed

        var testDescription: String { rawValue }

        @MainActor var cabal: Components.Schemas.Cabal {
            var cabal = Components.Schemas.Cabal.sample(role: "member", canVote: true)
            if self == .closed { cabal.status = CabalClosedModel.closedStatus }
            return cabal
        }

        @MainActor var pot: Components.Schemas.CabalPot {
            switch self {
            case .noShare: .noShare
            case .cashOnly: .sampleCashOnly
            case .member, .closed: .sampleInvested
            }
        }
    }

    private static func names(_ sections: [any CabalSection.Type]) -> [String] {
        sections.map { String(describing: $0) }
    }

    @Test func theTopLevelIsTheHeaderOrderThenTheTabs() {
        #expect(
            Self.names(CabalScreen.sections) == [
                "CabalHeaderSlot", "CabalClosedSlot", "CabalPauseSlot", "CabalJoinSlot", "CabalPotSlot",
                "CabalSliceSlot", "CabalActionsSlot", "CabalEmptySlot", "CabalStartersSlot", "CabalProposalsSlot",
                "CabalTabs",
            ])
    }

    @Test func eachTabHostsItsSlots() {
        #expect(Self.names(CabalTabs.sections(for: .treasury)) == ["CabalTreasuryTabSlot"])
        #expect(Self.names(CabalTabs.sections(for: .rules)) == ["CabalRulesSlot"])
        #expect(Self.names(CabalTabs.sections(for: .members)) == ["CabalMembersSlot"])
        #expect(Self.names(CabalTabs.sections(for: .voters)) == ["CabalVotersSlot"])
    }

    @Test func theTabSlotsAndTheChatPreviewLeaveTheTopLevel() {
        let top = Self.names(CabalScreen.sections)
        let moved = CabalTab.allCases.flatMap { Self.names(CabalTabs.sections(for: $0)) } + ["CabalChatPreviewSlot"]
        #expect(moved.allSatisfy { !top.contains($0) })
    }

    @Test func theMoreMenuOffersChatEditTreasuryAddressAndLeave() {
        #expect(
            CabalMoreItem.items(role: "creator", status: "active") == [.chat, .edit, .treasury, .leave])
        #expect(CabalMoreItem.items(role: "member", status: "active") == [.chat, .treasury, .leave])
        #expect(CabalMoreItem.items(role: "member", status: CabalClosedModel.closedStatus) == [.treasury, .leave])
        #expect(CabalMoreItem.items(role: nil, status: "active").isEmpty)
        #expect(CabalMoreItem.items(role: "creator", status: "active").last == .leave)
    }

    @Test func aRouteOpensTreasuryUnlessItNamesATab() {
        #expect(CabalRoute(id: "cabal-1").tab == .treasury)
        #expect(CabalRoute(id: "cabal-1", tab: .members).tab == .members)
        #expect(CabalRoute(id: "cabal-1") != CabalRoute(id: "cabal-1", tab: .rules))
    }

    @Test func theTabsPinOnceTheirTopReachesTheBar() {
        #expect(!CabalTabsPin.isPinned(tabsTop: nil, barBottom: 103))
        #expect(!CabalTabsPin.isPinned(tabsTop: 480, barBottom: 103))
        #expect(CabalTabsPin.isPinned(tabsTop: 103, barBottom: 103))
        #expect(CabalTabsPin.isPinned(tabsTop: -60, barBottom: 103))
    }

    @Test(.timeLimit(.minutes(1)), arguments: [CabalTab.treasury, .rules, .members])
    func theScreenShowsTheFourTabsAndOpensOnTheRoutesTab(tab: CabalTab) async throws {
        let transport = StubTransport(routes: [
            Self.cabalPath: Array(repeating: .json(.ok, try Self.json(Variant.member.cabal)), count: 8),
            Self.potPath: Array(repeating: .json(.ok, try Self.json(Variant.member.pot)), count: 8),
            Self.myCabalsPath: Array(repeating: .json(.ok, "[]"), count: 8),
        ])
        let sections: [any CabalSection.Type] = [CabalHeaderSlot.self, CabalTabs.self]
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(
            hosting: CabalScreen(cabalID: "cabal-1", tab: tab, sections: sections), over: transport)
        defer { TestWindow.close(window) }

        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains("cabal-tab-members") }
        let identifiers = AccessibilityTree.identifiers(in: window)
        for shown in CabalTab.allCases {
            #expect(identifiers.contains("cabal-tab-\(shown.rawValue)"), "identifiers: \(identifiers)")
        }
        if tab == .rules {
            await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains("cabal-rules") }
            #expect(AccessibilityTree.identifiers(in: window).contains("cabal-rules"))
        }
    }

    @Test func theInlineTitleShowsOnceTheNameScrollsUnderTheBar() {
        #expect(!CabalHeaderTitle.isShown(nameBottom: nil, barBottom: 103))
        #expect(!CabalHeaderTitle.isShown(nameBottom: 240, barBottom: 103))
        #expect(!CabalHeaderTitle.isShown(nameBottom: 103.5, barBottom: 103))
        #expect(CabalHeaderTitle.isShown(nameBottom: 103, barBottom: 103))
        #expect(CabalHeaderTitle.isShown(nameBottom: -40, barBottom: 103))
    }

    @Test(.timeLimit(.minutes(1)), arguments: Variant.allCases)
    func theFirstScreenShowsTheActionsWithoutClipping(variant: Variant) async throws {
        let transport = StubTransport(routes: [
            Self.cabalPath: Array(repeating: .json(.ok, try Self.json(variant.cabal)), count: 8),
            Self.potPath: Array(repeating: .json(.ok, try Self.json(variant.pot)), count: 8),
            Self.myCabalsPath: Array(repeating: .json(.ok, "[]"), count: 8),
        ])
        let sections: [any CabalSection.Type] = [
            CabalHeaderSlot.self, CabalClosedSlot.self, CabalPotSlot.self, CabalActionsSlot.self,
        ]
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(
            hosting: CabalScreen(cabalID: "cabal-1", sections: sections), over: transport)
        defer { TestWindow.close(window) }

        let ready = (variant == .closed) ? "cabal-closed-row" : "cabal-action-propose"
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains(ready) }
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains("cabal-pot-value") }
        for _ in 0..<500 { await Task.yield() }

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.contains(ready), "identifiers: \(identifiers)")
        if variant == .closed {
            #expect(Self.actionIDs.allSatisfy { !identifiers.contains($0) }, "identifiers: \(identifiers)")
            return
        }
        let screenHeight = window.screen.bounds.height
        let frames = Self.frames(in: window)
        let potTop = try #require(frames["cabal-pot-value"], "identifiers: \(identifiers)").minY
        for id in Self.actionIDs {
            let frame = try #require(frames[id], "\(id) missing; identifiers: \(identifiers)")
            #expect(frame.minY >= potTop, "\(id) sits above the pot")
            #expect(frame.maxY <= screenHeight, "\(id) is clipped: \(frame), screen height \(screenHeight)")
            #expect(frame.width > 0 && frame.height > 0, "\(id) has no size: \(frame)")
        }
    }

    private static func json(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private static func frames(in root: NSObject) -> [String: CGRect] {
        var found: [String: CGRect] = [:]
        func visit(_ node: NSObject, depth: Int) {
            guard depth < 80 else { return }
            if node.responds(to: #selector(getter: UIAccessibilityIdentification.accessibilityIdentifier)),
                let identifier = node.value(forKey: "accessibilityIdentifier") as? String, !identifier.isEmpty,
                found[identifier] == nil
            {
                found[identifier] = node.accessibilityFrame
            }
            var children: [NSObject] = (node as? UIView)?.subviews ?? []
            if let elements = node.accessibilityElements as? [NSObject] {
                children += elements
            } else {
                let count = node.accessibilityElementCount()
                if count != NSNotFound, count > 0 {
                    children += (0..<count).compactMap { node.accessibilityElement(at: $0) as? NSObject }
                }
            }
            for child in children { visit(child, depth: depth + 1) }
        }
        visit(root, depth: 0)
        return found
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
