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
    private let id: String
    private let cabalID: String
    private let repository: ProposalsRepository
    private let hints: any HintSource
    private let refresher: HintRefresher
    private let submission = IdempotentSubmission()

    public init(id: String, cabalID: String, repository: ProposalsRepository, hints: any HintSource) {
        self.id = id
        self.cabalID = cabalID
        self.repository = repository
        self.hints = hints
        let hook = ProposalReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.load() }
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
        } catch {}
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
