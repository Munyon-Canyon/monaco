import Foundation
import MonacoAPI
import MonacoGovernance
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

    public func load() async {
        if pager.items.isEmpty {
            await pager.loadFirst()
        } else {
            await refresh()
        }
    }

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
        let waiting = needs.filter { $0.myBallot == nil && !votedThisSession.contains($0.id) }.count
        let header = NeedsVoteHeader(waiting: waiting)
        return CabalProposalsSection(title: header.title, count: header.count, proposals: proposals)
    }
}

public struct NeedsVoteHeader: Equatable, Sendable {
    public let title: String
    public let count: Int?

    public init(waiting: Int) {
        title = waiting > 0 ? ProposalFeedCopy.needsYourVote : ProposalFeedCopy.feedTitle
        count = waiting > 0 ? waiting : nil
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
        for model in loaded { await model.refresh() }
    }
}

extension ProposalSummary {
    public var isTradeInProgress: Bool { status == .passed && swap?.status != "failed" }

    public var routedSaleShareNanos: Int64? {
        guard kind == "sell", let network, network != .solana, let bestPrice, bestPrice.isSell,
            bestPrice.winner.network == network, bestPrice.winner.netShareNanos > 0
        else { return nil }
        return bestPrice.winner.netShareNanos
    }
}

public enum ProposalViewerRole: Equatable, Sendable {
    case voter, memberNotVoter, nonMember
}

public enum ProposalVoteSelection: Equatable, Sendable {
    case none
    case picked(ProposalBallot)
    case pending(ProposalBallot)
    case committed(ProposalBallot)
}

extension ProposalBallot {
    public var other: ProposalBallot { self == .yes ? .no : .yes }
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
    public private(set) var isVoting = false
    public private(set) var lastVote: CastOutcome?
    public private(set) var voteRefusal: VoteRefusal?
    public private(set) var undoRefusal: UndoRefusal?
    public var voteSelection: ProposalVoteSelection = .none
    public private(set) var priceMove: ProposalPriceMove?
    public let livePrice: LiveBestPriceModel
    private let id: String
    private var cabalID: String?
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let now: @Sendable () -> Date
    private let calendar: Calendar
    private let refresher: HintRefresher
    private let submission = IdempotentSubmission()
    private let retrySubmission = IdempotentSubmission()
    private let voting: ProposalVoteModel
    private var retriedSwapID: String?
    private let proposeService: any ProposeService
    @ObservationIgnored private var isVisible = false
    @ObservationIgnored private var isObserving = false

    public init(
        id: String, cabalID: String?, repository: ProposalsRepository, hints: any HintSource,
        sleep: @escaping @Sendable (Duration) async throws -> Void = { try await Task.sleep(for: $0) },
        now: @escaping @Sendable () -> Date = Date.init, calendar: Calendar = .current
    ) {
        self.id = id
        self.cabalID = cabalID
        self.repository = repository
        self.hints = hints
        self.now = now
        self.calendar = calendar
        voting = ProposalVoteModel(repository: repository)
        proposeService = LiveProposeService(api: repository.api)
        livePrice = LiveBestPriceModel(sleep: sleep) { cabalID in
            try await repository.priceComparison(cabalID: cabalID, proposalID: id)
        }
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.load() }
    }

    public convenience init(
        sample detail: Components.Schemas.ProposalDetail, members: [Components.Schemas.CabalMember],
        asset: Components.Schemas.AssetDetail, priceMove: ProposalPriceMove? = nil, repository: ProposalsRepository,
        hints: any HintSource, now: @escaping @Sendable () -> Date = Date.init, calendar: Calendar = .current
    ) {
        self.init(
            id: detail.id, cabalID: detail.cabalId, repository: repository, hints: hints, now: now,
            calendar: calendar)
        value = ProposalDetail(detail)
        self.members = members.map(ProposalMember.init)
        self.asset = ProposalAsset(asset)
        self.priceMove = priceMove
    }

    public func load() async {
        do {
            let detail = try await repository.detail(id: id)
            async let members = try? repository.members(cabalID: detail.summary.cabalID)
            async let asset = try? repository.asset(symbol: detail.summary.symbol)
            value = detail
            if cabalID == nil && isObserving { Task { await observe() } }
            cabalID = detail.summary.cabalID
            syncVoteSelection(with: voting.applying(detail.summary))
            voting.acknowledge(detail.summary)
            if let members = await members { self.members = members }
            if let asset = await asset { self.asset = asset }
            errorMessage = nil
            syncLivePrice()
            await loadPriceMove(for: detail.summary)
        } catch {
            errorMessage = ToastCopy.message(for: APIError(error))
        }
    }

    private func loadPriceMove(for summary: ProposalSummary) async {
        guard summary.swap?.isPriceMoved == true else {
            priceMove = nil
            return
        }
        let live = try? await repository.liveQuote(for: summary)
        priceMove = live.flatMap {
            ProposalPriceMove(votedQuote: summary.quoteOutAmount, currentQuote: $0, isSell: summary.kind == "sell")
        }
    }

    public func vote(_ choice: String) async -> Bool {
        guard !isVoting, let current = value?.summary, let side = ProposalBallot(rawValue: choice) else {
            return false
        }
        let previous = voting.applying(current).myBallot.flatMap(ProposalBallot.init(rawValue:))
        isVoting = true
        defer { isVoting = false }
        voteRefusal = nil
        voteSelection = .pending(side)
        let result = await voting.cast(choice, on: current)
        guard let result else {
            errorMessage = voting.errorMessage
            voteRefusal = voting.voteRefusal
            voteSelection = .picked(side)
            if voteRefusal?.reloads == true {
                await load()
                errorMessage = voting.errorMessage
            }
            return false
        }
        lastVote = CastOutcome(
            choice: side, status: result.status, tally: result.tally, undoUntil: result.undoUntil,
            myVote: result.myVote, changed: previous != nil && previous != side)
        voteSelection = .committed(side)
        await load()
        return true
    }

    public func undo() async -> Bool {
        guard !isVoting, let current = value?.summary else { return false }
        undoRefusal = nil
        guard await voting.undo(on: current) != nil else {
            errorMessage = voting.errorMessage
            undoRefusal = voting.undoRefusal
            if undoRefusal?.reloads == true {
                await load()
                errorMessage = voting.errorMessage
            }
            return false
        }
        lastVote = nil
        voteSelection = .none
        await load()
        return true
    }

    public func beginChange() {
        guard case .committed(let side) = voteSelection, value?.summary.canVote == true else { return }
        voteSelection = .picked(side.other)
    }

    public func canUndo(now: Date) -> Bool {
        guard let until = lastVote?.undoUntil, !(lastVote?.settled ?? false) else { return false }
        return until > now
    }

    public func viewerRole(viewerID: String?) -> ProposalViewerRole {
        if value?.summary.canVote == true { return .voter }
        guard let viewerID, members.contains(where: { $0.id == viewerID }) else { return .nonMember }
        return .memberNotVoter
    }

    public var statusFacts: [String] {
        guard let value else { return [] }
        return ProposalVoteCopy.statusFacts(
            tally: summary?.tally ?? value.summary.tally, replies: value.replyCount, expiresAt: value.summary.expiresAt,
            now: now())
    }

    public var statusLine: String? { value == nil ? nil : statusFacts.joined(separator: " · ") }

    public var settledFacts: [String]? {
        guard let vote = lastVote, vote.settled, let summary = value?.summary else { return nil }
        return ProposalVoteCopy.settledFacts(
            status: vote.status, tally: vote.tally, voted: vote.tally.yes + vote.tally.no, voters: vote.tally.voters,
            sharesEstimate: sharesEstimate, isSell: summary.kind == "sell")
    }

    public var settledLine: String? { settledFacts?.joined(separator: " · ") }

    public var decidingLine: String? {
        guard let vote = lastVote, vote.settled, let myVote = vote.myVote else { return nil }
        return ProposalVoteCopy.decidingLine(myVote: myVote, needed: vote.tally.needed, voters: vote.tally.voters)
    }

    private var sharesEstimate: String? {
        guard let value = stats?.cells.first(where: { $0.label == "Shares" })?.value,
            value != ProposalStats.missing
        else { return nil }
        return value.hasPrefix("≈ ") ? String(value.dropFirst(2)) : value
    }

    private func syncVoteSelection(with summary: ProposalSummary) {
        switch voteSelection {
        case .pending: return
        case .picked: guard !(summary.status == .open && summary.canVote) else { return }
        case .none, .committed: break
        }
        voteSelection = summary.myBallot.flatMap(ProposalBallot.init(rawValue:)).map { .committed($0) } ?? .none
    }

    public var summary: ProposalSummary? {
        guard let current = value?.summary else { return nil }
        return voting.applying(isRetryPending ? ProposalSummary(retrying: current) : current)
    }

    private var isRetryPending: Bool {
        guard let retriedSwapID, let current = value?.summary else { return false }
        return current.swap?.id == retriedSwapID && [.passed, .executionBlocked].contains(current.status)
    }

    public var canWithdraw: Bool { value?.summary.canWithdraw == true }

    public var stats: ProposalStats? {
        value.map { ProposalStats.make(detail: $0, asset: asset, now: now(), calendar: calendar) }
    }

    public var retryableSwapID: String? {
        guard !isRetryPending, let swap = value?.summary.swap, swap.retryable else { return nil }
        return swap.id
    }

    public var retriesAtCurrentPrice: Bool { value?.summary.swap?.isPriceMoved == true }

    public func canRetry(viewerID: String?) -> Bool {
        guard retryableSwapID != nil, let viewerID else { return false }
        return members.contains { $0.id == viewerID }
    }

    public func retry() async {
        guard let swapID = retryableSwapID, !isRetrying else { return }
        let atCurrentPrice = retriesAtCurrentPrice
        isRetrying = true
        didRetry = false
        defer { isRetrying = false }
        do {
            try await repository.retrySwap(id: swapID, atCurrentPrice: atCurrentPrice, submission: retrySubmission)
            retriedSwapID = swapID
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
        isObserving = true
        guard let cabalID = value?.summary.cabalID ?? cabalID else { return }
        await refresher.observe([
            hints.hints(matching: .cabal(id: cabalID, what: "proposal_updated")),
            hints.hints(matching: .cabal(id: cabalID, what: "swap_updated")),
        ])
    }
    public func setVisible(_ visible: Bool) {
        isVisible = visible
        refresher.setVisible(visible)
        syncLivePrice()
    }

    public func refresh() async {
        async let detail: Void = load()
        async let price: Void = livePrice.refresh()
        _ = await (detail, price)
    }

    private func syncLivePrice() {
        let summary = value?.summary
        let isLive = isVisible && summary.map(LiveBestPriceModel.isLive) == true
        livePrice.poll(cabalID: isLive ? summary?.cabalID : nil)
    }
}

extension ProposalDetailModel {
    public convenience init(id: String, repository: ProposalsRepository, hints: any HintSource) {
        self.init(id: id, cabalID: nil, repository: repository, hints: hints)
    }
}

@Observable
@MainActor
public final class PendingVotesModel {
    public enum Phase: Equatable, Sendable { case loading, loaded, failed }

    public private(set) var phase: Phase = .loading
    public private(set) var votes: [PendingVote] = []
    public private(set) var details: [String: ProposalDetail] = [:]
    public private(set) var pausedCabals: Set<String> = []
    public private(set) var assets: [String: ProposalAsset] = [:]
    public private(set) var members: [String: [ProposalMember]] = [:]
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let refresher: HintRefresher

    public var needsVote: [PendingVote] { votes.filter { details[$0.id]?.summary.isTradeInProgress != true } }
    public var inProgress: [PendingVote] { votes.filter { details[$0.id]?.summary.isTradeInProgress == true } }

    public func waitingCount(votedThisSession: Set<String>) -> Int {
        needsVote.filter { !votedThisSession.contains($0.id) && details[$0.id]?.summary.myBallot == nil }.count
    }

    public init(repository: ProposalsRepository, hints: any HintSource) {
        self.repository = repository
        self.hints = hints
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.load() }
    }

    public func load() async {
        do {
            let pending = try await repository.pendingVotes()
            let pendingIDs = Set(pending.map(\.id))
            let departed = votes.filter { !pendingIDs.contains($0.id) }
            let candidates = departed + pending
            async let fetchedDetails = details(for: candidates)
            async let pausedCandidates = pausedAmong(Set(candidates.map(\.cabalID)))
            let (fetched, paused) = await (fetchedDetails, pausedCandidates)
            let kept = departed.filter {
                guard let summary = fetched[$0.id]?.summary else { return false }
                return summary.isTradeInProgress || (summary.status == .open && summary.myBallot != nil)
            }
            let listed = kept + pending
            votes = listed
            details = fetched.filter { entry in listed.contains { $0.id == entry.key } }
            pausedCabals = paused.intersection(listed.map(\.cabalID))
            phase = .loaded
            await loadCardContext(for: fetched.values.map(\.summary))
        } catch {
            if phase != .loaded { phase = .failed }
        }
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

    private func loadCardContext(for summaries: [ProposalSummary]) async {
        let context = await repository.cardContext(
            symbols: Set(summaries.map(\.symbol)).subtracting(assets.keys),
            cabalIDs: Set(summaries.map(\.cabalID)).subtracting(members.keys))
        assets.merge(context.assets) { _, new in new }
        members.merge(context.members) { _, new in new }
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
        let streams = ["proposal_created", "proposal_updated", "swap_updated"]
            .map { hints.hints(matching: .anyCabal(what: $0)) }
        await refresher.observe(streams)
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
    public private(set) var hasLoaded = false
    private let cabalID: String
    private let repository: ProposalsRepository

    private enum Fetched: Sendable {
        case members([ProposalMember])
        case asset(String, ProposalAsset)
    }

    public init(cabalID: String, repository: ProposalsRepository) {
        self.cabalID = cabalID
        self.repository = repository
    }

    public func load(for proposals: [ProposalSummary]) async {
        guard !proposals.isEmpty else { return }
        let needsMembers = members.isEmpty
        let symbols = Set(proposals.map(\.symbol)).subtracting(assets.keys)
        let fetched = await withTaskGroup(of: Fetched?.self) { group in
            if needsMembers {
                group.addTask { (try? await self.repository.members(cabalID: self.cabalID)).map(Fetched.members) }
            }
            for symbol in symbols {
                group.addTask { (try? await self.repository.asset(symbol: symbol)).map { .asset(symbol, $0) } }
            }
            var results: [Fetched] = []
            for await result in group { if let result { results.append(result) } }
            return results
        }
        guard !Task.isCancelled else { return }
        for item in fetched {
            switch item {
            case .members(let people): members = people
            case .asset(let symbol, let asset): assets[symbol] = asset
            }
        }
        hasLoaded = true
    }
}
