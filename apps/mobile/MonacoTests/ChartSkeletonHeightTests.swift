import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ChartSkeletonHeightTests {
    private typealias MyHistory = Components.Schemas.MyPnlHistory
    private typealias CabalHistory = Components.Schemas.CabalValueHistory
    private typealias Replies = [String: StubTransport.Reply]

    private static let tolerance: CGFloat = 2
    private static let portfolioPath = "/v1/me/portfolio"
    private static let homeHistoryPath = "/v1/me/pnl-history?range=1D"
    private static let cabalHistoryPath = "/v1/cabals/cabal-1/value-history?range=1M"
    private static let cabalPath = "/v1/cabals/cabal-1"
    private static let cabalPotPath = "/v1/cabals/cabal-1/pot"

    @Test(.timeLimit(.minutes(1)))
    func theHomeHeroKeepsItsHeightThroughEveryStageOfLoading() async throws {
        let skeleton = try await Self.homeHeight(portfolio: .gate, history: .gate)
        let chartLoading = try await Self.homeHeight(
            portfolio: Self.reply(Components.Schemas.MyPortfolio.sample), history: .gate,
            waitingFor: "home-portfolio-total")
        let loaded = try await Self.homeHeight(withHistory: true)

        #expect(abs(chartLoading - skeleton) < Self.tolerance, "skeleton \(skeleton), chart loading \(chartLoading)")
        #expect(abs(loaded - skeleton) < Self.tolerance, "skeleton \(skeleton), loaded \(loaded)")
    }

    @Test(.timeLimit(.minutes(1)))
    func aHomeRangeWithoutHistoryTakesTheHeightOfOneWithHistory() async throws {
        let with = try await Self.homeHeight(withHistory: true)
        let without = try await Self.homeHeight(withHistory: false)

        #expect(abs(without - with) < Self.tolerance, "with history \(with), without \(without)")
    }

    @Test(.timeLimit(.minutes(1)))
    func theCabalChartKeepsItsHeightWhenItsHistoryLoads() async throws {
        let skeleton = try await Self.cabalHeight(history: .gate)
        let loaded = try await Self.cabalHeight(withHistory: true)

        #expect(abs(loaded - skeleton) < Self.tolerance, "skeleton \(skeleton), loaded \(loaded)")
    }

    @Test(.timeLimit(.minutes(1)))
    func aCabalRangeWithoutHistoryTakesTheHeightOfOneWithHistory() async throws {
        let with = try await Self.cabalHeight(withHistory: true)
        let without = try await Self.cabalHeight(withHistory: false)

        #expect(abs(without - with) < Self.tolerance, "with history \(with), without \(without)")
    }

    @Test(.timeLimit(.minutes(1)))
    func theCabalHeroKeepsItsHeightWhenTheCabalAndPotLoad() async throws {
        let skeleton = try await Self.cabalHeroHeight(cabal: .gate, pot: .gate)
        let loaded = try await Self.cabalHeroHeight(
            cabal: Self.reply(Components.Schemas.Cabal.sample(role: "member")),
            pot: Self.reply(Components.Schemas.CabalPot.sampleInvested),
            waitingFor: "cabal-pot-split")

        #expect(abs(loaded - skeleton) < Self.tolerance, "skeleton \(skeleton), loaded \(loaded)")
    }

    private static func homeHeight(withHistory: Bool) async throws -> CGFloat {
        var history = MyHistory.sample(range: ._1d)
        if !withHistory { history.points = [] }
        return try await homeHeight(
            portfolio: reply(Components.Schemas.MyPortfolio.sample), history: reply(history),
            waitingFor: withHistory ? "home-pnl-chart" : "home-portfolio-short")
    }

    private static func homeHeight(
        portfolio: StubTransport.Reply, history: StubTransport.Reply, waitingFor marker: String? = nil
    ) async throws -> CGFloat {
        try await height(
            of: HomeScreen(sections: [HomePortfolioSlot.self]),
            replies: [portfolioPath: portfolio, homeHistoryPath: history], waitingFor: marker)
    }

    private static func cabalHeight(withHistory: Bool) async throws -> CGFloat {
        var history = CabalHistory.sample(cabalID: "cabal-1")
        if !withHistory { history.points = [] }
        return try await cabalHeight(
            history: reply(history), waitingFor: withHistory ? "cabal-value-chart" : "cabal-value-chart-short")
    }

    private static func cabalHeight(
        history: StubTransport.Reply, waitingFor marker: String? = nil
    ) async throws -> CGFloat {
        try await height(
            of: CabalValueChartSlot.body(for: CabalContext(cabalID: "cabal-1")),
            replies: [cabalHistoryPath: history], waitingFor: marker)
    }

    private static func cabalHeroHeight(
        cabal: StubTransport.Reply, pot: StubTransport.Reply, waitingFor marker: String? = nil
    ) async throws -> CGFloat {
        try await height(
            of: CabalHeroHost(), replies: [cabalPath: cabal, cabalPotPath: pot], waitingFor: marker,
            andShowing: marker == nil ? nil : "cabal-header")
    }

    private static func height(
        of view: some View, replies: Replies, waitingFor marker: String?, andShowing other: String? = nil
    ) async throws -> CGFloat {
        let transport = StubTransport(routes: replies.mapValues { [$0] })
        let hosted = try Hosted(view, over: transport)
        defer { hosted.close() }
        await transport.waitForRequests(replies.count)
        if let marker { await hosted.until(marker) }
        if let other { await hosted.until(other) }
        return hosted.height
    }

    private static func reply(_ value: some Encodable) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(value), as: UTF8.self))
    }
}

private struct CabalHeroHost: View {
    private let context = CabalContext(cabalID: "cabal-1")
    @Environment(AppEnvironment.self) private var environment
    @State private var cabal: CabalModel?

    var body: some View {
        SectionStackLayout(spacing: SectionStackMetrics.spacing) {
            CabalHeaderSlot.body(for: context)
            CabalPotSlot.body(for: context)
        }
        .environment(\.cabalModel, cabal)
        .task {
            let model = CabalModel(cabalID: context.cabalID, api: environment.api, hints: environment.hints)
            cabal = model
            await model.load()
        }
    }
}

@MainActor
private final class Hosted {
    private static let width: CGFloat = 390

    private let window: UIWindow
    private let controller: UIHostingController<AnyView>

    init(_ view: some View, over transport: StubTransport) throws {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        try AccessibilityTree.setAutomation(enabled: true)
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let environment = AppEnvironment(
            auth: auth, api: api, hints: FakeHintSource(), isAuthenticated: { true }, endAuthSession: {})
        controller = UIHostingController(rootView: AnyView(view.environment(environment).environment(ToastCenter())))
        window = UIWindow(windowScene: scene)
        window.rootViewController = controller
        window.makeKeyAndVisible()
    }

    var height: CGFloat {
        controller.sizeThatFits(in: CGSize(width: Self.width, height: .greatestFiniteMagnitude)).height
    }

    func until(_ identifier: String) async {
        await ProposalTestWindow.until { shows(identifier) }
        #expect(shows(identifier), "the view never showed \(identifier)")
    }

    private func shows(_ identifier: String) -> Bool {
        AccessibilityTree.identifiers(in: window).contains(identifier)
    }

    func close() {
        window.isHidden = true
        try? AccessibilityTree.setAutomation(enabled: false)
    }
}
