import MonacoAPI
import Observation

@Observable
@MainActor
public final class ProposalListModel {
    public let pager: CursorPager<ProposalSummary>
    private let hints: any HintSource
    private let refresher: HintRefresher

    public init(cabalID: String, filter: ProposalFilter, repository: ProposalsRepository, hints: any HintSource) {
        self.hints = hints
        pager = CursorPager { try await repository.list(cabalID: cabalID, filter: filter, cursor: $0) }
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak pager] in await pager?.refreshFirstPage() }
    }

    public func load() async { await pager.loadFirst() }
    public func observe(cabalID: String) async {
        await refresher.observe(hints.hints(matching: .cabal(id: cabalID, what: "proposal_created")))
    }
    public func setVisible(_ visible: Bool) { refresher.setVisible(visible) }
}

@Observable
@MainActor
public final class ProposalDetailModel {
    public private(set) var value: ProposalDetail?
    public private(set) var members: [ProposalMember] = []
    public private(set) var asset: ProposalAsset?
    public private(set) var errorMessage: String?
    public private(set) var didWithdraw = false
    private let id: String
    private let cabalID: String
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let refresher: HintRefresher
    private let submission = IdempotentSubmission()
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
        await refresher.observe(hints.hints(matching: .cabal(id: cabalID, what: "proposal_updated")))
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

    public init(repository: ProposalsRepository, hints: any HintSource) {
        self.repository = repository
        self.hints = hints
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.load() }
    }

    public func load() async {
        do {
            votes = try await repository.pendingVotes()
            details = Dictionary(
                uniqueKeysWithValues: await withTaskGroup(
                    of: (String, ProposalDetail?).self
                ) { group in
                    for vote in votes {
                        group.addTask {
                            (vote.id, try? await self.repository.detail(id: vote.id))
                        }
                    }
                    var pairs: [(String, ProposalDetail)] = []
                    for await (id, detail) in group {
                        if let detail { pairs.append((id, detail)) }
                    }
                    return pairs
                })
            pausedCabals = await pausedAmong(Set(votes.map(\.cabalID)))
        } catch {}
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
