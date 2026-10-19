import Foundation
import MonacoAPI
import Observation

public struct ChatProposalFacts: Equatable, Sendable {
    public enum Phase: Equatable, Sendable {
        case open
        case approved
        case closed
    }

    public let proposalID: String
    public let proposerID: String
    public let assetName: String
    public let isSell: Bool
    public let amountMicros: Int64?
    public let createdAt: Date
    public let phase: Phase

    public init(
        proposalID: String, proposerID: String, assetName: String, isSell: Bool, amountMicros: Int64?,
        createdAt: Date, phase: Phase
    ) {
        self.proposalID = proposalID
        self.proposerID = proposerID
        self.assetName = assetName
        self.isSell = isSell
        self.amountMicros = amountMicros
        self.createdAt = createdAt
        self.phase = phase
    }

    public init(_ detail: ProposalDetail, asset: ProposalAsset?) {
        let summary = detail.summary
        self.init(
            proposalID: summary.id, proposerID: summary.proposerID,
            assetName: asset?.displayName ?? AssetSymbolFormatter.display(summary.symbol),
            isSell: summary.kind == "sell", amountMicros: summary.usdcMicros ?? summary.quoteOutAmount,
            createdAt: summary.createdAt, phase: Self.phase(summary.status))
    }

    private static func phase(_ status: ProposalStatus) -> Phase {
        switch status {
        case .open: .open
        case .passed: .approved
        case .executed, .failed, .expired, .withdrawn, .voided, .executionBlocked: .closed
        }
    }

    public func title(proposer: String) -> String {
        phase == .approved ? GroupChatCopy.approved : GroupChatCopy.proposed(proposer, assetName, isSell: isSell)
    }

    public var amountLine: String? {
        amountMicros.map { GroupChatCopy.fromPot(UsdAmountFormatter.format(micros: $0)) }
    }

    public var buttonTitle: String {
        phase == .open ? GroupChatCopy.review : GroupChatCopy.status
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
            return ChatProposalFacts(detail, asset: asset)
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
}
