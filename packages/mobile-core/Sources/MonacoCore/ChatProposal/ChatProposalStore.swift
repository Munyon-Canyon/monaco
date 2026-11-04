import Foundation
import MonacoAPI
import Observation

public struct ChatProposalFacts: Equatable, Sendable {
    public let summary: ProposalSummary
    public let asset: ProposalAsset?

    public init(summary: ProposalSummary, asset: ProposalAsset?) {
        self.summary = summary
        self.asset = asset
    }
}

public enum ChatProposalState: Equatable, Sendable {
    case loading
    case loaded(ChatProposalFacts)
    case failed
}

@Observable
@MainActor
public final class ChatProposalStore {
    public typealias Fetch = @Sendable (String) async throws -> ChatProposalFacts

    private let fetch: Fetch
    private var states: [String: ChatProposalState] = [:]
    @ObservationIgnored private var inFlight: Set<String> = []

    public init(fetch: @escaping Fetch) {
        self.fetch = fetch
    }

    public convenience init(api: APIClient) {
        let repository = ProposalsRepository(api: api)
        self.init { id in
            let detail = try await repository.detail(id: id)
            let asset = try? await repository.asset(symbol: detail.summary.symbol)
            return ChatProposalFacts(summary: detail.summary, asset: asset)
        }
    }

    public func state(for id: String) -> ChatProposalState {
        states[id] ?? .loading
    }

    public func load(_ id: String) async {
        if case .loaded = states[id] { return }
        guard inFlight.insert(id).inserted else { return }
        defer { inFlight.remove(id) }
        do {
            states[id] = .loaded(try await fetch(id))
        } catch {
            guard !Task.isCancelled else { return }
            states[id] = .failed
        }
    }

    public func retry(_ id: String) async {
        guard case .failed = states[id] else { return }
        states[id] = .loading
        await load(id)
    }

    public func reload(_ ids: [String]) async {
        for id in ids {
            guard case .loaded = states[id] else { continue }
            guard let facts = try? await fetch(id) else { continue }
            states[id] = .loaded(facts)
        }
    }

    public func observe(cabalID: String, hints: any HintSource) async {
        for await hint in hints.hints(matching: .cabal(id: cabalID, what: "proposal_updated")) {
            switch hint {
            case .changed(_, _, let id): await reload([id])
            case .resync: await reload(Array(states.keys))
            }
        }
    }
}
