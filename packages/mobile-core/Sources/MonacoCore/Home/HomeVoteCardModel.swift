import Foundation
import MonacoAPI
import MonacoGovernance
import Observation

public struct HomePendingVote: Identifiable, Equatable, Sendable {
    public let id: String
    public let cabalID: String
    public let cabalName: String
    public let kind: String
    public let rule: String?
    public let symbol: String?
    public let assetName: String?
    public let logoURL: URL?
    public let usdcMicros: Int64?
    public let tokenAmount: Int64?
    public let tokenDecimals: Int?
    public let tally: ProposalTally
    public let replyCount: Int
    public let expiresAt: Date

    public static let ruleKind = "rule"

    public var isRule: Bool { kind == Self.ruleKind }
    public var isSell: Bool { kind == "sell" }

    public init(
        id: String, cabalID: String, cabalName: String, kind: String, rule: String? = nil, symbol: String? = nil,
        assetName: String? = nil, logoURL: URL? = nil, usdcMicros: Int64? = nil, tokenAmount: Int64? = nil,
        tokenDecimals: Int? = nil, tally: ProposalTally, replyCount: Int, expiresAt: Date
    ) {
        self.id = id
        self.cabalID = cabalID
        self.cabalName = cabalName
        self.kind = kind
        self.rule = rule
        self.symbol = symbol
        self.assetName = assetName
        self.logoURL = logoURL
        self.usdcMicros = usdcMicros
        self.tokenAmount = tokenAmount
        self.tokenDecimals = tokenDecimals
        self.tally = tally
        self.replyCount = replyCount
        self.expiresAt = expiresAt
    }

    init(_ value: Components.Schemas.PendingVote) {
        self.init(
            id: value.proposalId, cabalID: value.cabalId, cabalName: value.cabalName, kind: value.kind.rawValue,
            rule: value.ruleChange?.rule.rawValue, symbol: value.symbol, assetName: value.assetName,
            logoURL: value.assetLogoUrl.flatMap(URL.init(string:)), usdcMicros: value.usdcMicros,
            tokenAmount: value.tokenAmount, tokenDecimals: value.tokenDecimals, tally: ProposalTally(value.tally),
            replyCount: value.replyCount, expiresAt: value.expiresAt)
    }
}

@Observable
@MainActor
public final class HomeVoteCardModel: Identifiable {
    public enum Phase: Equatable, Sendable {
        case idle
        case picked(ProposalBallot)
        case holding(ProposalBallot, progress: Double)
        case committing(ProposalBallot)
        case committed(CastOutcome)
        case failed(String)
    }

    public static let defaultReleaseAfter: TimeInterval = 10

    public nonisolated var id: String { proposalID }
    public let proposalID: String
    public private(set) var row: HomePendingVote
    public private(set) var tally: ProposalTally
    public private(set) var replyCount: Int
    public private(set) var phase: Phase = .idle
    public private(set) var sharesEstimate: String?
    public private(set) var committedAt: Date?
    public private(set) var errorMessage: String?
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let now: @Sendable () -> Date
    private let releaseAfter: TimeInterval
    private var submission = IdempotentSubmission()
    private var retraction = IdempotentSubmission()
    private var isSubmitting = false
    public var onCommitted: (@MainActor (CastOutcome) -> Void)?

    public init(
        api: APIClient, hints: any HintSource, row: HomePendingVote, now: @escaping @Sendable () -> Date = Date.init,
        releaseAfter: TimeInterval = HomeVoteCardModel.defaultReleaseAfter
    ) {
        proposalID = row.id
        self.row = row
        tally = row.tally
        replyCount = row.replyCount
        repository = ProposalsRepository(api: api)
        self.hints = hints
        self.now = now
        self.releaseAfter = releaseAfter
    }

    public var isCommitted: Bool {
        if case .committed = phase { return true }
        return false
    }

    public var outcome: CastOutcome? {
        if case .committed(let outcome) = phase { return outcome }
        return nil
    }

    public var isSettled: Bool { outcome?.settled == true }

    public var canUndo: Bool {
        guard let outcome, !outcome.settled, let until = outcome.undoUntil else { return false }
        return until > now()
    }

    public var selectedSide: ProposalBallot? {
        switch phase {
        case .picked(let side), .holding(let side, _), .committing(let side): side
        case .committed(let outcome): outcome.choice
        case .idle, .failed: nil
        }
    }

    public var isInteractive: Bool {
        switch phase {
        case .idle, .picked, .holding, .failed: true
        case .committing, .committed: false
        }
    }

    public func select(_ side: ProposalBallot?) {
        guard isInteractive else { return }
        phase = side.map { .picked($0) } ?? .idle
    }

    public func hold(_ side: ProposalBallot, progress: Double) {
        guard isInteractive else { return }
        phase = .holding(side, progress: min(max(progress, 0), 1))
    }

    public func lift() {
        guard case .holding(let side, _) = phase else { return }
        phase = .picked(side)
    }

    public func beginCommit(_ side: ProposalBallot) {
        guard isInteractive else { return }
        phase = .committing(side)
    }

    public func dismissFailure() {
        guard case .failed = phase else { return }
        phase = .idle
    }

    @discardableResult
    public func vote(_ side: ProposalBallot) async -> CastOutcome? {
        guard !isSubmitting, !isCommitted else { return nil }
        isSubmitting = true
        defer { isSubmitting = false }
        errorMessage = nil
        phase = .committing(side)
        do {
            let result = try await repository.vote(id: proposalID, choice: side.rawValue, submission: submission)
            submission = IdempotentSubmission()
            tally = result.tally
            let outcome = CastOutcome(
                choice: side, status: result.status, tally: result.tally, undoUntil: result.undoUntil,
                myVote: result.myVote)
            committedAt = now()
            phase = .committed(outcome)
            onCommitted?(outcome)
            if result.status == .passed, !row.isRule { await loadSettlement() }
            return outcome
        } catch {
            let refusal = VoteRefusal(APIError(error))
            errorMessage = refusal.message
            phase = .failed(refusal.message)
            if refusal.reloads { await reload() }
            return nil
        }
    }

    @discardableResult
    public func undo() async -> Bool {
        guard canUndo else { return false }
        errorMessage = nil
        do {
            let result = try await repository.retractVote(id: proposalID, submission: retraction)
            retraction = IdempotentSubmission()
            tally = result.tally
            phase = .idle
            committedAt = nil
            sharesEstimate = nil
            return true
        } catch {
            errorMessage = UndoRefusal(APIError(error)).message
            return false
        }
    }

    public func update(_ row: HomePendingVote) {
        self.row = row
        guard isInteractive else { return }
        tally = row.tally
        replyCount = row.replyCount
    }

    public func releaseAt() -> Date? {
        guard let committedAt else { return nil }
        let hold = committedAt.addingTimeInterval(releaseAfter)
        guard let until = outcome?.undoUntil, !isSettled else { return hold }
        return max(hold, until)
    }

    public func reload() async {
        guard let detail = try? await repository.detail(id: proposalID) else { return }
        if !isCommitted {
            tally = detail.summary.tally
            replyCount = detail.replyCount
        }
    }

    public func observe() async {
        let id = proposalID
        let stream = hints.hints(matching: .cabal(id: row.cabalID, what: "proposal_updated"))
        for await hint in stream {
            guard case .changed(_, _, let hinted) = hint, hinted == id else { continue }
            await reload()
        }
    }

    private func loadSettlement() async {
        guard let detail = try? await repository.detail(id: proposalID) else { return }
        replyCount = detail.replyCount
        let stats = ProposalStats.make(detail: detail, asset: nil, now: now(), calendar: .current)
        guard let value = stats.cells.first(where: { $0.label == "Shares" })?.value, value != ProposalStats.missing
        else { return }
        sharesEstimate = value.hasPrefix("≈ ") ? String(value.dropFirst(2)) : value
    }
}

extension HomeVoteCardModel {
    public static func ruleTitle(_ rule: String?) -> String {
        switch rule {
        case "show_sides": "Show who voted what"
        case "max_trade_bps": "Max per trade"
        case "threshold": "Votes to pass"
        case "proposal_expiry_seconds": "Vote window"
        case "join_mode": "Who can join"
        case "voter": "Who can vote"
        case "admin": "Who can manage"
        case "creator": "Who leads the cabal"
        default: "Cabal rule"
        }
    }

    public var headline: String {
        if row.isRule { return "Rule change \u{00B7} \(Self.ruleTitle(row.rule))" }
        let ticker = AssetSymbolFormatter.display(row.symbol ?? "")
        let amount: String? =
            row.isSell
            ? row.tokenAmount.map { Self.shares($0, decimals: row.tokenDecimals) }
            : row.usdcMicros.map(CabalHeaderCopy.compactDollars)
        let name = "\(row.isSell ? "Sell" : "Buy") \(ticker)"
        return [name, amount].compactMap { $0 }.joined(separator: " \u{00B7} ")
    }

    private static func shares(_ atomics: Int64, decimals: Int?) -> String {
        TokenQuantityFormatter.label(
            fromAtomics: String(atomics), decimals: decimals ?? AssetCatalogDefaults.decimals, kind: .stock)
    }

    public func subhead(now: Date) -> String {
        var facts = [row.cabalName]
        if let outcome, outcome.settled {
            facts.append(contentsOf: settledFacts(outcome))
        } else {
            var live = ProposalVoteCopy.statusFacts(
                tally: tally, replies: replyCount, expiresAt: row.expiresAt, now: now)
            if let last = live.last, last.hasPrefix("ends in ") { live[live.count - 1] = String(last.dropFirst(8)) }
            facts.append(contentsOf: live)
        }
        return facts.joined(separator: " \u{00B7} ")
    }

    private func settledFacts(_ outcome: CastOutcome) -> [String] {
        var facts = ProposalVoteCopy.settledFacts(
            status: outcome.status, tally: outcome.tally, voted: outcome.tally.yes + outcome.tally.no,
            voters: outcome.tally.voters, sharesEstimate: sharesEstimate, isSell: row.isSell)
        if facts.count > 1 { facts.remove(at: 1) }
        if let first = facts.first { facts[0] = first.prefix(1).lowercased() + first.dropFirst() }
        return facts
    }

    public func hint(holding: Bool) -> String? {
        switch phase {
        case .picked: ProposalVoteCopy.hint(holding ? .holding : .picked)
        case .holding: ProposalVoteCopy.hint(.holding)
        case .committed(let outcome): committedLine(outcome)
        case .idle, .committing, .failed: nil
        }
    }

    public func committedLine(_ outcome: CastOutcome) -> String? {
        guard let myVote = outcome.myVote else { return nil }
        if outcome.settled {
            return ProposalVoteCopy.decidingLine(
                myVote: myVote, needed: outcome.tally.needed, voters: outcome.tally.voters)
        }
        let head = "Yours was the \(ProposalVoteCopy.ordinal(myVote.order)) vote"
        guard outcome.choice == .yes else { return head }
        let remaining = max(outcome.tally.needed - outcome.tally.yes, 1)
        return "\(head) \u{00B7} \(remaining) more yes \(remaining == 1 ? "passes" : "pass") it"
    }
}

#if DEBUG
extension HomeVoteCardModel {
    public func preview(_ phase: Phase, sharesEstimate: String? = nil) {
        self.phase = phase
        self.sharesEstimate = sharesEstimate
        if case .committed(let outcome) = phase {
            tally = outcome.tally
            committedAt = now()
        }
    }
}
#endif
