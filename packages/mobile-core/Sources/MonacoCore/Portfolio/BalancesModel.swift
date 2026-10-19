import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class BalancesModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case failed
        case loaded
    }

    public struct Row: Equatable, Identifiable, Sendable {
        public let id: String
        public let name: String
        public let pictureURL: String?
        public let value: String
    }

    public static let unavailable = "—"

    public private(set) var toast: String?

    private let portfolio: PortfolioModel
    private let balance: BalanceSource

    public init(portfolio: PortfolioModel, balance: BalanceSource) {
        self.portfolio = portfolio
        self.balance = balance
    }

    public var phase: Phase {
        switch (portfolio.state, balance.state) {
        case (.failed, _), (_, .failed): .failed
        case (.loaded, .loaded): .loaded
        case (.idle, _), (.loading, _), (_, .idle), (_, .loading): .loading
        }
    }

    public var netWorthMicros: Int64? {
        guard let summary = portfolio.summary, let account = balance.balance else { return nil }
        return summary.netWorthMicros(cashMicros: account.availableMicros)
    }

    public var netWorth: String { Self.format(netWorthMicros) }

    public var inCabals: String { Self.format(portfolio.summary?.totalMicros) }

    public var cash: String { Self.format(balance.balance?.availableMicros) }

    public var allocation: [Row] {
        (portfolio.summary?.rows ?? []).map {
            Row(
                id: $0.id, name: $0.name, pictureURL: $0.pictureURL,
                value: UsdAmountFormatter.format(flooredMicros: $0.valueMicros))
        }
    }

    public var isCashOnly: Bool { portfolio.summary?.rows.isEmpty == true }

    public func load() async {
        await read()
    }

    public func refresh() async {
        await read()
    }

    public func dismissToast() {
        toast = nil
    }

    private func read() async {
        let failures = balance.failureTick
        async let portfolioRead: Void = portfolio.load()
        async let balanceRead: Void = balance.load()
        _ = await (portfolioRead, balanceRead)
        noteFailures(since: failures)
    }

    private func noteFailures(since failures: Int) {
        if let message = portfolio.toast {
            portfolio.dismissToast()
            toast = message
        } else if balance.failureTick != failures, balance.balance != nil, let error = balance.lastError,
            !BackgroundRefresh.isActive
        {
            toast = BalanceSource.message(for: error)
        }
    }

    private static func format(_ micros: Int64?) -> String {
        micros.map { UsdAmountFormatter.format(flooredMicros: $0) } ?? unavailable
    }
}
