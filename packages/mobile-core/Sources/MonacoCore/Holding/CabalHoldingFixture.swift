#if DEBUG
import Foundation
import MonacoAPI

public enum CabalHoldingFixture {
    public enum Scenario: String, CaseIterable, Sendable {
        case ready
        case loss
        case empty
        case loading
        case stale
        case error
        case notMember
        case productionGaps
        case tradesFailed
    }

    public static let cabalID = "00000000-0000-4000-8000-000000000a01"
    public static let symbol = "NVDAx"
    public static let cabalName = "Big Ballers"
    public static let assetName = "NVIDIA"

    private static let micros: Int64 = 1_000_000

    public static func figures(loss: Bool, now: Date) -> CabalHolding.Figures {
        let end: Int64 = (loss ? 5_412 : 6_600) * micros
        let moves: [(HoldingPeriod, Int64, TimeInterval)] = [
            (.oneWeek, loss ? 180 : 150, 7),
            (.oneMonth, loss ? -663 : 525, 30),
            (.threeMonths, loss ? 410 : 900, 90),
            (.oneYear, loss ? -1_020 : 1_500, 365),
            (.all, loss ? -780 : 407, 540),
        ]
        var series: [HoldingPeriod: ValueCurve] = [:]
        for (period, move, days) in moves {
            let start = end - move * micros
            series[period] = curve(from: start, to: end, days: days, now: now)
        }
        return CabalHolding.Figures(
            averageBuyMicros: 206_430_000, averageSellMicros: 230_000_000,
            realizedMicros: loss ? -42_300_000 : 117_860_000, series: series)
    }

    public static func holding(_ scenario: Scenario, now: Date) -> CabalHolding {
        let loss = scenario == .loss
        let empty = scenario == .empty
        return CabalHolding(
            cabalID: cabalID, symbol: symbol, assetName: assetName, cabalName: cabalName,
            valueMicros: empty ? 0 : (loss ? 5_412 : 6_600) * micros,
            costBasisMicros: empty ? 0 : 6_192_860_000,
            pnlMicros: empty ? 0 : (loss ? -780_860_000 : 407_140_000),
            unitsText: empty ? "0" : "30.0000",
            staleMinutes: scenario == .stale ? 14 : nil,
            figures: scenario == .productionGaps ? .none : (empty ? .none : figures(loss: loss, now: now)))
    }

    public static let trades: [HoldingTrade] = [
        HoldingTrade(
            id: "trade-3", isBuy: false, unitsText: "8.0000", dateText: "Oct 12, 2026 · 14:02 UTC",
            unitPriceText: "$230.00", totalMicros: 1_840 * micros),
        HoldingTrade(
            id: "trade-2", isBuy: true, unitsText: "13.5556", dateText: "Oct 02, 2026 · 09:41 UTC",
            unitPriceText: "$213.00", totalMicros: 2_887_390_000),
        HoldingTrade(
            id: "trade-1", isBuy: true, unitsText: "24.4444", dateText: "Sep 25, 2026 · 10:18 UTC",
            unitPriceText: "$200.00", totalMicros: 4_888_880_000),
    ]

    @MainActor
    public static func model(_ scenario: Scenario, now: Date) -> CabalHoldingModel {
        let model = CabalHoldingModel(
            cabalID: cabalID, symbol: symbol,
            api: APIClient(
                serverURL: URL(fileURLWithPath: "/"), tokens: FixtureTokens(), transport: UnreachableTransport()),
            now: { now })
        switch scenario {
        case .loading:
            model.outcome = .loading
            model.trades = .loading
        case .error:
            model.outcome = .failed(.transport(URLError(.notConnectedToInternet)))
            model.trades = .loading
        case .notMember:
            model.outcome = .notMember
            model.cabalName = cabalName
            model.trades = .loading
        case .empty:
            model.outcome = .empty(holding(scenario, now: now))
            model.trades = .loaded([], hasMore: false)
            model.proposals = proposals()
            model.members = members()
        case .ready, .loss, .stale, .productionGaps, .tradesFailed:
            model.outcome = .loaded(holding(scenario, now: now))
            model.trades = scenario == .tradesFailed ? .failed : .loaded(trades, hasMore: true)
            model.proposals = proposals()
            model.members = members()
        }
        return model
    }

    static func members() -> [ProposalMember] {
        Components.Schemas.Cabal.sampleWithMembers(role: "member").members.map(ProposalMember.init)
    }

    static func proposals() -> [ProposalSummary] {
        let proposers = members()
        var proposal = Components.Schemas.Proposal.sample()
        proposal.id = "00000000-0000-4000-8000-000000000a51"
        proposal.cabalId = cabalID
        proposal.proposerId = proposers.first?.id ?? proposal.proposerId
        proposal.symbol = symbol
        proposal.usdcMicros = 1_200 * micros
        return [ProposalSummary(proposal)]
    }

    private static func curve(from start: Int64, to end: Int64, days: TimeInterval, now: Date) -> ValueCurve {
        let count = 32
        let span = end - start
        let samples = (0..<count).map { index -> ValueCurve.Sample in
            let remaining = Int64(count - 1 - index)
            let wave = index == 0 ? 0 : Int64(index * 7 % 5 - 2)
            let swing = wave * abs(span) / 60 * remaining / Int64(count - 1)
            let value = start + span * Int64(index) / Int64(count - 1) + swing
            let age = Int64(days * 86_400) * remaining / Int64(count - 1)
            return ValueCurve.Sample(at: now.addingTimeInterval(-TimeInterval(age)), value: value, pnl: value - start)
        }
        return ValueCurve(samples: samples)
    }
}

private struct UnreachableTransport: ClientTransport {
    func send(
        _: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        throw URLError(.notConnectedToInternet)
    }
}

private struct FixtureTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "fixture-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}
#endif
