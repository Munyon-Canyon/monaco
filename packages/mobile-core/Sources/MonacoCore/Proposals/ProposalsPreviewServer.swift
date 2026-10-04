#if DEBUG
import Foundation
import MonacoAPI

public actor ProposalsPreviewServer: ClientTransport {
    public private(set) var requests: [String] = []
    private var cabal: Components.Schemas.Cabal
    private var proposals: [Components.Schemas.ProposalDetail]
    private var assets: [String: Components.Schemas.AssetDetail]
    private let viewerID: String
    private var failing = false

    public init(
        cabal: Components.Schemas.Cabal = .sampleWithMembers(role: "member"),
        proposals: [Components.Schemas.ProposalDetail],
        assets: [Components.Schemas.AssetDetail] = [.proposalSample()],
        viewerID: String = Components.Schemas.ProposalDetail.sampleVoterIDs[0]
    ) {
        self.cabal = cabal
        self.proposals = proposals
        self.assets = Dictionary(assets.map { ($0.symbol, $0) }, uniquingKeysWith: { first, _ in first })
        self.viewerID = viewerID
    }

    public func setFailing(_ failing: Bool) { self.failing = failing }

    public func replace(_ proposal: Components.Schemas.ProposalDetail) {
        guard let index = proposals.firstIndex(where: { $0.id == proposal.id }) else {
            proposals.insert(proposal, at: 0)
            return
        }
        proposals[index] = proposal
    }

    public func count(_ prefix: String) -> Int {
        requests.filter { $0.hasPrefix(prefix) }.count
    }

    public func send(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let target = request.path ?? ""
        requests.append("\(request.method.rawValue) \(target)")
        if failing { return try problem(503, ._internal, "Monaco is busy. Try again.") }
        let path = target.split(separator: "?").first.map(String.init) ?? target
        let parts = path.split(separator: "/").map(String.init)
        switch (request.method, parts.count, parts.dropFirst().first, parts.last) {
        case (.get, 3, "cabals", _):
            return try ok(cabal)
        case (.get, 4, "cabals", "proposals"):
            let open = target.contains("filter=open")
            let page = proposals.filter { ($0.status == .open) == open }.map(\.summary)
            return try ok(Components.Schemas.ProposalList(proposals: page, nextCursor: nil))
        case (.get, 3, "proposals", let id?):
            guard let proposal = proposals.first(where: { $0.id == id }) else {
                return try problem(404, .proposalNotFound, "That proposal is gone.")
            }
            return try ok(proposal)
        case (.post, 4, "proposals", "votes"):
            let choice = try await JSONDecoder().decode(
                Components.Schemas.CastVoteRequest.self, from: Data(collecting: body ?? HTTPBody(), upTo: 4096)
            ).choice
            return try ok(vote(parts[2], choice))
        case (.get, 3, "me", "pending-votes"):
            let pending = proposals.filter { proposal in
                proposal.status == .open && proposal.voters.contains { $0.userId == viewerID && $0.choice == nil }
            }
            return try ok(
                pending.sorted { $0.expiresAt < $1.expiresAt }.map {
                    Components.Schemas.PendingVote(
                        proposalId: $0.id, cabalId: $0.cabalId, kind: $0.kind, symbol: $0.symbol,
                        expiresAt: $0.expiresAt)
                })
        case (.get, 3, "assets", let symbol?):
            guard let asset = assets[symbol] else { return try problem(404, .assetNotFound, "Unknown asset.") }
            return try ok(asset)
        default:
            return try problem(404, .notFound, "No preview route for \(path).")
        }
    }

    private func vote(_ id: String, _ choice: Components.Schemas.BallotChoice) -> Components.Schemas.VoteResult {
        guard var proposal = proposals.first(where: { $0.id == id }) else {
            let empty = Components.Schemas.Tally(yes: 0, no: 0, voters: 0, needed: 0)
            return .init(proposalId: id, status: .open, tally: empty, myBallot: choice)
        }
        let wire = Components.Schemas.ProposalVoter.ChoicePayload(rawValue: choice.rawValue)
        proposal.voters = proposal.voters.map { voter in
            voter.userId == viewerID ? .init(userId: voter.userId, choice: wire, castAt: voter.castAt) : voter
        }
        let yes = proposal.voters.filter { $0.choice == .yes }.count
        let no = proposal.voters.filter { $0.choice == .no }.count
        proposal.tally = .init(yes: yes, no: no, voters: proposal.voters.count, needed: proposal.tally.needed)
        proposal.myBallot = .init(rawValue: choice.rawValue)
        replace(proposal)
        return .init(proposalId: id, status: proposal.status, tally: proposal.tally, myBallot: choice)
    }

    private func ok(_ value: some Encodable) throws -> (HTTPResponse, HTTPBody?) {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(try encoder.encode(value)))
    }

    private func problem(_ status: Int, _ code: Components.Schemas.ErrorCode, _ message: String) throws -> (
        HTTPResponse, HTTPBody?
    ) {
        let problem = Components.Schemas.Problem(
            _type: .about_colon_blank, title: message, status: status, code: code, message: message,
            traceId: "preview", retryable: false)
        var response = HTTPResponse(status: .init(code: status))
        response.headerFields[.contentType] = "application/problem+json"
        return (response, HTTPBody(try JSONEncoder().encode(problem)))
    }
}

extension ProposalsRepository {
    public static func preview(_ server: ProposalsPreviewServer) -> ProposalsRepository {
        ProposalsRepository(
            api: APIClient(
                serverURL: URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/"),
                tokens: ProposalsPreviewTokens(),
                transport: server))
    }
}

public struct ProposalsPreviewHints: HintSource {
    public init() {}

    public func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { _ in }
    }
}

private struct ProposalsPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}
#endif
