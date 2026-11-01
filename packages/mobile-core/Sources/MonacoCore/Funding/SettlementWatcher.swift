import Foundation
import MonacoAPI
import Observation

public struct SettlementTicket: Codable, Equatable, Sendable {
    public enum Kind: String, Codable, Sendable {
        case fund, withdraw
    }

    public let kind: Kind
    public let resourceID: String
    public let cabalID: String?
    public let cabalName: String?
    public let fullBalanceLabel: String?

    public init(
        kind: Kind, resourceID: String, cabalID: String? = nil, cabalName: String? = nil,
        fullBalanceLabel: String? = nil
    ) {
        self.kind = kind
        self.resourceID = resourceID
        self.cabalID = cabalID
        self.cabalName = cabalName
        self.fullBalanceLabel = fullBalanceLabel
    }

    var slot: String { "settle:\(kind.rawValue):\(resourceID)" }
}

public struct SettlementNotice: Equatable, Sendable {
    public enum Outcome: Equatable, Sendable {
        case fund(FundProgress)
        case withdraw(WithdrawProgress)
    }

    public let ticket: SettlementTicket
    public let outcome: Outcome
}

@Observable
@MainActor
public final class SettlementWatcher {
    public private(set) var fund: [String: FundProgress] = [:]
    public private(set) var withdrawals: [String: WithdrawProgress] = [:]
    public private(set) var notice: SettlementNotice?

    private let fundSource: FundSource
    private let withdrawSource: WithdrawSource
    private let hints: any HintSource
    private let journal: PendingIntentJournal
    private let schedule: ReconcileSchedule
    @ObservationIgnored private var accountID: String?
    @ObservationIgnored private var tasks: [String: Task<Void, Never>] = [:]
    @ObservationIgnored private var onScreen: Set<String> = []

    public init(
        api: APIClient, hints: any HintSource, journal: PendingIntentJournal,
        schedule: ReconcileSchedule = .standard()
    ) {
        fundSource = FundSource(api: api)
        withdrawSource = WithdrawSource(api: api)
        self.hints = hints
        self.journal = journal
        self.schedule = schedule
    }

    public var watchedCount: Int { tasks.count }

    public func bind(accountID: String) {
        guard self.accountID != accountID else { return }
        reset()
        self.accountID = accountID
        for intent in journal.intents(accountID: accountID) where intent.slot.hasPrefix(Self.slotPrefix) {
            guard let data = intent.context, let ticket = try? JSONDecoder().decode(SettlementTicket.self, from: data)
            else { continue }
            follow(ticket)
        }
    }

    public func watch(_ ticket: SettlementTicket) {
        if let accountID {
            journal.save(
                PendingIntent(
                    accountID: accountID, slot: ticket.slot, context: try? JSONEncoder().encode(ticket),
                    resourceID: ticket.resourceID))
        }
        follow(ticket)
    }

    public func present(_ resourceID: String) {
        onScreen.insert(resourceID)
    }

    public func dismiss(_ resourceID: String) {
        onScreen.remove(resourceID)
    }

    public func reset() {
        for task in tasks.values { task.cancel() }
        tasks = [:]
        fund = [:]
        withdrawals = [:]
        notice = nil
        onScreen = []
        accountID = nil
    }

    private static let slotPrefix = "settle:"

    private func follow(_ ticket: SettlementTicket) {
        guard tasks[ticket.slot] == nil else { return }
        let account = accountID
        tasks[ticket.slot] = Task { [weak self] in
            guard let self else { return }
            let outcome = await self.settle(ticket)
            guard !Task.isCancelled, let outcome else { return }
            self.tasks[ticket.slot] = nil
            if let account { self.journal.remove(accountID: account, slot: ticket.slot) }
            if self.isShownOnScreen(outcome, ticket: ticket) { return }
            self.notice = SettlementNotice(ticket: ticket, outcome: outcome)
        }
    }

    private func isShownOnScreen(_ outcome: SettlementNotice.Outcome, ticket: SettlementTicket) -> Bool {
        guard onScreen.contains(ticket.resourceID) else { return false }
        switch outcome {
        case .fund(.failed), .withdraw(.failed): return true
        case .fund, .withdraw: return false
        }
    }

    private func settle(_ ticket: SettlementTicket) async -> SettlementNotice.Outcome? {
        switch ticket.kind {
        case .fund:
            guard let cabalID = ticket.cabalID else { return nil }
            let funding = Funding(
                cabalID: cabalID, source: fundSource, hints: hints, schedule: schedule)
            funding.resume(transferID: ticket.resourceID)
            fund[ticket.resourceID] = funding.progress
            let settled = await funding.settle()
            guard settled.isSettled else { return nil }
            fund[ticket.resourceID] = settled
            return .fund(settled)
        case .withdraw:
            let withdrawing = Withdrawing(
                source: withdrawSource, hints: hints, schedule: schedule)
            withdrawing.resume(withdrawalID: ticket.resourceID)
            withdrawals[ticket.resourceID] = withdrawing.progress
            let settled = await withdrawing.settle()
            guard settled.isSettled else { return nil }
            withdrawals[ticket.resourceID] = settled
            return .withdraw(settled)
        }
    }
}
