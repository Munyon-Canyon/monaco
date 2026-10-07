import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class ProposalListModel {
    public let pager: CursorPager<ProposalSummary>
    public let filter: ProposalFilter
    public var onOutcome: (@MainActor (ProposalOutcome) -> Void)?
    private let hints: any HintSource
    private let refresher: HintRefresher

    public init(cabalID: String, filter: ProposalFilter, repository: ProposalsRepository, hints: any HintSource) {
        self.hints = hints
        self.filter = filter
        pager = CursorPager { try await repository.list(cabalID: cabalID, filter: filter, cursor: $0) }
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.refresh() }
    }

    public func load() async { await pager.loadFirst() }

    public func refresh() async {
        let before = Dictionary(pager.items.map { ($0.id, $0.status) }, uniquingKeysWith: { first, _ in first })
        await pager.refreshFirstPage()
        pager.remove { !filter.includes($0.status) }
        for proposal in pager.items {
            if let outcome = ProposalOutcome(from: before[proposal.id], to: proposal) { onOutcome?(outcome) }
        }
    }
    public func observe(cabalID: String) async {
        await refresher.observe([
            hints.hints(matching: .cabal(id: cabalID, what: "proposal_created")),
            hints.hints(matching: .cabal(id: cabalID, what: "proposal_updated")),
            hints.hints(matching: .cabal(id: cabalID, what: "swap_updated")),
        ])
    }
    public func setVisible(_ visible: Bool) { refresher.setVisible(visible) }

    public func needsVote(votedThisSession: Set<String>) -> [ProposalSummary] {
        pager.items.filter {
            $0.status == .open && $0.canVote && ($0.myBallot == nil || votedThisSession.contains($0.id))
        }
    }

    public var trading: [ProposalSummary] { pager.items.filter(\.isTradeInProgress) }

    public func recentOutcomes(now: Date) -> [ProposalSummary] {
        pager.items.filter { $0.isRecentOutcome(now: now) }
    }

    public func cabalSection(votedThisSession: Set<String>) -> CabalProposalsSection? {
        let needs = needsVote(votedThisSession: votedThisSession)
        let needsIDs = Set(needs.map(\.id))
        let voted = pager.items.filter { $0.status == .open && $0.myBallot != nil && !needsIDs.contains($0.id) }
        let proposals = needs + voted
        guard !proposals.isEmpty else { return nil }
        return CabalProposalsSection(
            title: needs.isEmpty ? "Proposals" : "Needs your vote", count: needs.isEmpty ? nil : needs.count,
            proposals: proposals)
    }
}

public struct CabalProposalsSection: Equatable, Sendable {
    public let title: String
    public let count: Int?
    public let proposals: [ProposalSummary]
}

public enum ProposalSegment: String, CaseIterable, Identifiable, Sendable {
    case open, passed, executed, failed

    public var id: String { rawValue }
    public var title: String { rawValue.capitalized }
    public var filter: ProposalFilter { ProposalFilter(rawValue: rawValue) ?? .all }
    public var emptyTitle: String { "No \(rawValue) proposals yet" }
}

@Observable
@MainActor
public final class ProposalSegmentsModel {
    public var selected: ProposalSegment = .open
    private var models: [ProposalSegment: ProposalListModel] = [:]
    private let cabalID: String
    private let repository: ProposalsRepository
    private let hints: any HintSource

    public init(cabalID: String, repository: ProposalsRepository, hints: any HintSource) {
        self.cabalID = cabalID
        self.repository = repository
        self.hints = hints
    }

    public func model(for segment: ProposalSegment) -> ProposalListModel {
        if let existing = models[segment] { return existing }
        let created = ProposalListModel(
            cabalID: cabalID, filter: segment.filter, repository: repository, hints: hints)
        models[segment] = created
        return created
    }

    public var loaded: [ProposalListModel] { ProposalSegment.allCases.compactMap { models[$0] } }

    public func refreshLoaded() async {
        for model in loaded { await model.pager.loadFirst() }
    }
}

extension ProposalSummary {
    public var isTradeInProgress: Bool { status == .passed && swap?.status != "failed" }
}

@Observable
@MainActor
public final class ProposalDetailModel {
    public private(set) var value: ProposalDetail?
    public private(set) var members: [ProposalMember] = []
    public private(set) var asset: ProposalAsset?
    public private(set) var errorMessage: String?
    public private(set) var didWithdraw = false
    public private(set) var isRetrying = false
    public private(set) var didRetry = false
    private let id: String
    private let cabalID: String
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let refresher: HintRefresher
    private let submission = IdempotentSubmission()
    private let retrySubmission = IdempotentSubmission()
    private let proposeService: any ProposeService

    public init(id: String, cabalID: String, repository: ProposalsRepository, hints: any HintSource) {
        self.id = id
        self.cabalID = cabalID
        self.repository = repository
        self.hints = hints
        proposeService = LiveProposeService(api: repository.api)
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.load() }
    }

    public convenience init(
        sample detail: Components.Schemas.ProposalDetail, members: [Components.Schemas.CabalMember],
        asset: Components.Schemas.AssetDetail, repository: ProposalsRepository, hints: any HintSource
    ) {
        self.init(id: detail.id, cabalID: detail.cabalId, repository: repository, hints: hints)
        value = ProposalDetail(detail)
        self.members = members.map(ProposalMember.init)
        self.asset = ProposalAsset(asset)
    }

    public func load() async {
        do {
            let detail = try await repository.detail(id: id)
            async let members = try? repository.members(cabalID: detail.summary.cabalID)
            async let asset = try? repository.asset(symbol: detail.summary.symbol)
            value = detail
            self.members = await members ?? []
            self.asset = await asset
            errorMessage = nil
        } catch {
            errorMessage = ToastCopy.message(for: APIError(error))
        }
    }
    public func vote(_ choice: String) async {
        do {
            try await repository.vote(id: id, choice: choice, submission: submission)
            await load()
        } catch {
            errorMessage = ToastCopy.message(for: APIError(error))
        }
    }

    public var canWithdraw: Bool { value?.summary.canWithdraw == true }

    public var retryableSwapID: String? {
        guard let swap = value?.summary.swap, swap.retryable else { return nil }
        return swap.id
    }

    public func retry() async {
        guard let swapID = retryableSwapID, !isRetrying else { return }
        isRetrying = true
        didRetry = false
        defer { isRetrying = false }
        do {
            try await repository.retrySwap(id: swapID, submission: retrySubmission)
            didRetry = true
            errorMessage = nil
            await load()
        } catch {
            errorMessage = ToastCopy.message(for: APIError(error))
        }
    }

    public func withdraw() async {
        didWithdraw = false
        do {
            try await proposeService.withdraw(proposalID: id, submission: submission)
            didWithdraw = true
            errorMessage = nil
            await load()
        } catch {
            errorMessage = ToastCopy.message(for: APIError(error))
        }
    }

    public func observe() async {
        let cabalID = value?.summary.cabalID ?? cabalID
        await refresher.observe([
            hints.hints(matching: .cabal(id: cabalID, what: "proposal_updated")),
            hints.hints(matching: .cabal(id: cabalID, what: "swap_updated")),
        ])
    }
    public func setVisible(_ visible: Bool) { refresher.setVisible(visible) }
}

@Observable
@MainActor
public final class PendingVotesModel {
    public private(set) var votes: [PendingVote] = []
    public private(set) var details: [String: ProposalDetail] = [:]
    public private(set) var pausedCabals: Set<String> = []
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let refresher: HintRefresher

    public var needsVote: [PendingVote] { votes.filter { details[$0.id]?.summary.isTradeInProgress != true } }
    public var inProgress: [PendingVote] { votes.filter { details[$0.id]?.summary.isTradeInProgress == true } }

    public init(repository: ProposalsRepository, hints: any HintSource) {
        self.repository = repository
        self.hints = hints
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.load() }
    }

    public func load(keeping voted: Set<String> = []) async {
        do {
            let pending = try await repository.pendingVotes()
            let pendingIDs = Set(pending.map(\.id))
            let departed = votes.filter { !pendingIDs.contains($0.id) }
            let fetched = await details(for: departed + pending)
            let kept = departed.filter {
                guard let summary = fetched[$0.id]?.summary else { return false }
                return summary.isTradeInProgress || (summary.status == .open && voted.contains($0.id))
            }
            votes = kept + pending
            details = fetched.filter { entry in votes.contains { $0.id == entry.key } }
            pausedCabals = await pausedAmong(Set(votes.map(\.cabalID)))
        } catch {}
    }

    private func details(for votes: [PendingVote]) async -> [String: ProposalDetail] {
        await withTaskGroup(of: (String, ProposalDetail?).self) { group in
            for vote in votes {
                group.addTask { (vote.id, try? await self.repository.detail(id: vote.id)) }
            }
            var pairs: [String: ProposalDetail] = [:]
            for await (id, detail) in group {
                if let detail { pairs[id] = detail }
            }
            return pairs
        }
    }

    private func pausedAmong(_ cabalIDs: Set<String>) async -> Set<String> {
        await withTaskGroup(of: String?.self) { group in
            for id in cabalIDs {
                group.addTask { (try? await self.repository.isPaused(cabalID: id)) == true ? id : nil }
            }
            var paused: Set<String> = []
            for await id in group { if let id { paused.insert(id) } }
            return paused
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: nil)))
    }
    public func setVisible(_ visible: Bool) { refresher.setVisible(visible) }
}

@MainActor
private final class ProposalReloadHook {
    var run: (@MainActor () async -> Void)?
}

@Observable
@MainActor
public final class ProposalCardContext {
    public private(set) var members: [ProposalMember] = []
    public private(set) var assets: [String: ProposalAsset] = [:]
    private let cabalID: String
    private let repository: ProposalsRepository

    public init(cabalID: String, repository: ProposalsRepository) {
        self.cabalID = cabalID
        self.repository = repository
    }

    public func load(for proposals: [ProposalSummary]) async {
        guard !proposals.isEmpty else { return }
        if members.isEmpty { members = (try? await repository.members(cabalID: cabalID)) ?? [] }
        for symbol in Set(proposals.map(\.symbol)).subtracting(assets.keys).sorted() {
            if let asset = try? await repository.asset(symbol: symbol) { assets[symbol] = asset }
        }
    }
}
