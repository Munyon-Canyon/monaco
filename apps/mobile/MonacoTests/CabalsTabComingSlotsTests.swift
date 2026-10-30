import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct CabalsTabComingSlotsTests {
    @Test func theFourSectionsAreLive() {
        let sections = CabalsTabScreen.sections
        #expect(sections.count == 4)
        #expect(sections.allSatisfy { $0.isLive })
        #expect(sections.contains { $0 == CabalsJoinSlot.self })
        #expect(sections.contains { $0 == CabalsVoteSlot.self })
        #expect(sections.contains { $0 == CabalsGridSlot.self })
        #expect(sections.contains { $0 == CabalsShareListSlot.self })
    }

    @Test func theOldListInvitesAndBoardSlotsAreGoneFromTheTab() {
        let sections = CabalsTabScreen.sections
        #expect(!sections.contains { $0 == CabalsInvitesSlot.self })
        #expect(!sections.contains { $0 == CabalsBoardSlot.self })
    }

    @Test func invitesLeadOnlyWhenTheGridPassesThreeRows() {
        #expect(!CabalsGridMetrics.pinsInvitesFirst(tiles: 3, columns: 4))
        #expect(!CabalsGridMetrics.pinsInvitesFirst(tiles: 12, columns: 4))
        #expect(CabalsGridMetrics.pinsInvitesFirst(tiles: 13, columns: 4))
        #expect(CabalsGridMetrics.pinsInvitesFirst(tiles: 149, columns: 4))
        #expect(!CabalsGridMetrics.pinsInvitesFirst(tiles: 6, columns: 2))
        #expect(CabalsGridMetrics.pinsInvitesFirst(tiles: 7, columns: 2))
    }

    @Test(.timeLimit(.minutes(1)))
    func emptyTabOffersToStartACabal() async throws {
        let labels = try await Self.labels(
            restricted: false, portfolio: .sampleEmpty, invites: "[]", until: "Your cabal starts here")

        #expect(labels.contains("Create a cabal"))
        #expect(labels.contains("Explore cabals"))
        #expect(labels.contains("Create a cabal or open a shared invitation."))
    }

    @Test(.timeLimit(.minutes(1)))
    func restrictedEmptyTabHasNoActionToStartACabal() async throws {
        let labels = try await Self.labels(
            restricted: true, portfolio: .sampleEmpty, invites: "[]", until: "Your cabal starts here")

        #expect(!labels.contains("Create a cabal"))
        #expect(!labels.contains("Explore cabals"))
        #expect(labels.contains("Open a shared invitation to join a cabal."))
    }

    @Test(.timeLimit(.minutes(1)))
    func theGridDrawsEachCabalAndTheInvitationAndTheShareList() async throws {
        let labels = try await Self.labels(
            restricted: false, portfolio: .sampleOwnProfile, invites: Self.quantKids, until: "Quant Kids, invited")

        #expect(labels.contains("AC Capital, up 4.7 percent"))
        #expect(labels.contains("Tech Titans, down 1.7 percent"))
        #expect(labels.contains("Quant Kids, invited"))
        #expect(labels.contains("New cabal"))
        #expect(labels.contains { $0.contains("Hold a cabal for actions") })
        #expect(labels.contains { $0.contains("$22,580 treasury · 12% yours") })
    }

    @Test(.timeLimit(.minutes(1)))
    func aJoinedButUnfundedCabalIsATileNotAnInvitation() async throws {
        var portfolio = Components.Schemas.MyPortfolio.sampleLoss
        portfolio.cabals = Array(portfolio.cabals.suffix(1))

        let labels = try await Self.labels(
            restricted: false, portfolio: portfolio, invites: "[]", until: "Sunday Investors, not funded")

        #expect(labels.contains("Sunday Investors, not funded"))
        #expect(!labels.contains { $0.hasSuffix("invited") })
    }

    private static let quantKids = """
        [{"request_id":"01890a5d-ac96-774b-bcce-b302099a8059","cabal":{"id":"01890a5d-ac96-774b-bcce-b302099a805d","name":"Quant Kids","picture_url":null,"member_count":5},"invited_by":{"user_id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"kai","display_name":"Kai","photo_url":null},"expires_at":"2026-10-09T15:00:00Z"}]
        """

    private static func labels(
        restricted: Bool, portfolio: Components.Schemas.MyPortfolio, invites: String, until marker: String
    ) async throws -> [String] {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let body = String(decoding: try encoder.encode(portfolio), as: UTF8.self)
        let transport = StubTransport(routes: [
            "/v1/me/portfolio": [.json(.ok, body)],
            "/v1/me/cabal-invites": [.json(.ok, invites)],
            "/v1/me/pending-votes": [.json(.ok, "[]")],
        ])
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(), api: api, hints: FakeHintSource(),
            isAuthenticated: { true }, endAuthSession: {})
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = UIWindow(windowScene: scene)
        window.frame = CGRect(x: 0, y: 0, width: 390, height: 1200)
        window.rootViewController = UIHostingController(
            rootView: CabalsTabScreen()
                .environment(environment)
                .environment(ToastCenter())
                .environment(\.accountRestricted, restricted)
        )
        window.makeKeyAndVisible()
        defer { TestWindow.close(window) }
        window.layoutIfNeeded()
        var labels: [String] = []
        var settled = false
        while !settled {
            try Task.checkCancellation()
            await withCheckedContinuation { continuation in
                RunLoop.main.perform { continuation.resume() }
            }
            let next = accessibilityLabels(of: window)
            settled = next == labels && next.contains { $0.contains(marker) }
            labels = next
        }
        return labels
    }

    private static func accessibilityLabels(of root: NSObject) -> [String] {
        var labels: [String] = []
        if let label = root.accessibilityLabel, !label.isEmpty { labels.append(label) }
        if let children = accessibilityChildren(of: root) {
            for child in children { labels += accessibilityLabels(of: child) }
        } else if let view = root as? UIView {
            for subview in view.subviews { labels += accessibilityLabels(of: subview) }
        }
        return labels
    }

    private static func accessibilityChildren(of node: NSObject) -> [NSObject]? {
        if let elements = node.accessibilityElements { return elements.compactMap { $0 as? NSObject } }
        let count = node.accessibilityElementCount()
        guard count != NSNotFound, count > 0 else { return nil }
        return (0..<count).compactMap { node.accessibilityElement(at: $0) as? NSObject }
    }
}
