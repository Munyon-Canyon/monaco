import Foundation
import MonacoAPI

public struct ProposalSwap: Equatable, Sendable {
    public let status: String
    public let id: String?
    public let failureCode: String?
    public let failureMessage: String?
    public let retryable: Bool
    public let usdcMicros: Int64?
    public let tokenAmount: Int64?
    public let tokenDecimals: Int?
    public let priceMicros: Int64?
    public let feeMicros: Int64?
    public let filledAt: Date?

    public static let priceMovedCode = "price_moved"
    public var isPriceMoved: Bool { status == "failed" && failureCode == Self.priceMovedCode }

    init(_ swap: Components.Schemas.ProposalDetail.SwapPayload?) {
        status = swap?.status.rawValue ?? ""
        id = swap?.swapId
        failureCode = swap?.failureCode
        failureMessage = swap?.failureMessage
        retryable = swap?.retryable ?? false
        usdcMicros = swap?.usdcMicros
        tokenAmount = swap?.tokenAmount
        tokenDecimals = swap?.tokenDecimals
        priceMicros = swap?.priceMicros
        feeMicros = swap?.feeMicros
        filledAt = swap?.filledAt
    }

    public func executionFigures(kind: AssetKind) -> ProposalExecutionFigures? {
        guard status == "confirmed", let priceMicros, let tokenAmount, let tokenDecimals,
            let quantity = TokenQuantityFormatter.quantity(fromAtomics: String(tokenAmount), decimals: tokenDecimals)
        else { return nil }
        return ProposalExecutionFigures(
            price: UsdAmountFormatter.format(micros: priceMicros),
            units: TokenQuantityFormatter.label(quantity: quantity, kind: kind))
    }
}

public struct ProposalExecutionFigures: Equatable, Sendable {
    public let price: String
    public let units: String

    public init(price: String, units: String) {
        self.price = price
        self.units = units
    }
}

public struct ProposalSummary: Identifiable, Equatable, Sendable {
    public let id: String
    public let cabalID: String
    public let cabalName: String
    public let kind: String
    public let symbol: String
    public let proposerID: String
    public let thesis: String?
    public let usdcMicros: Int64?
    public let tokenAmount: Int64?
    public let quoteOutAmount: Int64
    public let createdAt: Date
    public let tally: ProposalTally
    public let myBallot: String?
    public let status: ProposalStatus
    public let statusMessage: String?
    public let expiresAt: Date
    public let canVote: Bool
    public let canWithdraw: Bool
    public let swap: ProposalSwap?
    public let bestPrice: BestPrice?
    public let network: Components.Schemas.Network?

    init(_ other: ProposalSummary, ballot: String, tally: ProposalTally) {
        self = other.replacing(ballot: ballot, tally: tally)
    }

    init(retrying other: ProposalSummary) {
        self.init(
            id: other.id, cabalID: other.cabalID, cabalName: other.cabalName, kind: other.kind,
            symbol: other.symbol, proposerID: other.proposerID, thesis: other.thesis, usdcMicros: other.usdcMicros,
            tokenAmount: other.tokenAmount, quoteOutAmount: other.quoteOutAmount, createdAt: other.createdAt,
            tally: other.tally, myBallot: other.myBallot, status: .passed, statusMessage: other.statusMessage,
            expiresAt: other.expiresAt, canVote: other.canVote, canWithdraw: other.canWithdraw, swap: nil,
            bestPrice: other.bestPrice, network: other.network)
    }

    private func replacing(ballot: String, tally: ProposalTally) -> ProposalSummary {
        ProposalSummary(
            id: id, cabalID: cabalID, cabalName: cabalName, kind: kind, symbol: symbol, proposerID: proposerID,
            thesis: thesis, usdcMicros: usdcMicros, tokenAmount: tokenAmount, quoteOutAmount: quoteOutAmount,
            createdAt: createdAt, tally: tally, myBallot: ballot, status: status, statusMessage: statusMessage,
            expiresAt: expiresAt,
            canVote: canVote, canWithdraw: canWithdraw, swap: swap, bestPrice: bestPrice, network: network)
    }

    private init(
        id: String, cabalID: String, cabalName: String, kind: String, symbol: String, proposerID: String,
        thesis: String?, usdcMicros: Int64?, tokenAmount: Int64?, quoteOutAmount: Int64, createdAt: Date,
        tally: ProposalTally, myBallot: String?, status: ProposalStatus, statusMessage: String?, expiresAt: Date,
        canVote: Bool,
        canWithdraw: Bool, swap: ProposalSwap?, bestPrice: BestPrice?, network: Components.Schemas.Network?
    ) {
        self.id = id
        self.cabalID = cabalID
        self.cabalName = cabalName
        self.kind = kind
        self.symbol = symbol
        self.proposerID = proposerID
        self.thesis = thesis
        self.usdcMicros = usdcMicros
        self.tokenAmount = tokenAmount
        self.quoteOutAmount = quoteOutAmount
        self.createdAt = createdAt
        self.tally = tally
        self.myBallot = myBallot
        self.status = status
        self.statusMessage = statusMessage
        self.expiresAt = expiresAt
        self.canVote = canVote
        self.canWithdraw = canWithdraw
        self.swap = swap
        self.bestPrice = bestPrice
        self.network = network
    }

    init(
        _ proposal: Components.Schemas.Proposal,
        canVote: Bool? = nil,
        canWithdraw: Bool = false,
        swap: ProposalSwap? = nil
    ) {
        id = proposal.id
        cabalID = proposal.cabalId
        cabalName = proposal.cabalName
        kind = proposal.kind.rawValue
        symbol = proposal.symbol
        proposerID = proposal.proposerId
        thesis = proposal.thesis
        usdcMicros = proposal.usdcMicros
        tokenAmount = proposal.tokenAmount
        quoteOutAmount = proposal.quoteOutAmount
        createdAt = proposal.createdAt
        tally = ProposalTally(proposal.tally)
        myBallot = proposal.myBallot?.rawValue
        status = ProposalStatus(proposal.status)
        statusMessage = proposal.statusMessage
        expiresAt = proposal.expiresAt
        self.canVote = canVote ?? proposal.canVote
        self.canWithdraw = canWithdraw
        self.swap = swap
        bestPrice = BestPrice(proposal.priceComparison, isSell: proposal.kind == .sell)
        network = proposal.network
    }
}

public struct ProposalStatsData: Equatable, Sendable {
    public let priceMicros: Int64?
    public let sharesAtomics: Int64?
    public let sharesExact: Bool
    public let tokenDecimals: Int
    public let treasuryShareBps: Int64?
    public let closesAt: Date
    public let closedAt: Date?
    public let sinceBps: Int64?

    public init(
        priceMicros: Int64? = nil, sharesAtomics: Int64? = nil, sharesExact: Bool = false, tokenDecimals: Int = 0,
        treasuryShareBps: Int64? = nil, closesAt: Date, closedAt: Date? = nil, sinceBps: Int64? = nil
    ) {
        self.priceMicros = priceMicros
        self.sharesAtomics = sharesAtomics
        self.sharesExact = sharesExact
        self.tokenDecimals = tokenDecimals
        self.treasuryShareBps = treasuryShareBps
        self.closesAt = closesAt
        self.closedAt = closedAt
        self.sinceBps = sinceBps
    }

    init(_ value: Components.Schemas.ProposalStats) {
        self.init(
            priceMicros: value.priceMicros, sharesAtomics: value.sharesAtomics,
            sharesExact: value.sharesExact ?? false, tokenDecimals: value.tokenDecimals ?? 0,
            treasuryShareBps: value.treasuryShareBps, closesAt: value.closesAt, closedAt: value.closedAt,
            sinceBps: value.sinceBps)
    }
}

public struct ProposalMyVote: Equatable, Sendable {
    public let order: Int
    public let of: Int
    public let yesOrder: Int
    public let noOrder: Int
    public let decided: Bool

    public init(order: Int, of: Int, yesOrder: Int, noOrder: Int, decided: Bool) {
        self.order = order
        self.of = of
        self.yesOrder = yesOrder
        self.noOrder = noOrder
        self.decided = decided
    }

    init(_ value: Components.Schemas.ProposalDetail.MyVotePayload) {
        self.init(
            order: value.order, of: value.of, yesOrder: value.yesOrder, noOrder: value.noOrder,
            decided: value.decided)
    }

    init(_ value: Components.Schemas.MyVote) {
        self.init(
            order: value.order, of: value.of, yesOrder: value.yesOrder, noOrder: value.noOrder,
            decided: value.decided)
    }
}

public struct ProposalDetail: Identifiable, Equatable, Sendable {
    public let summary: ProposalSummary
    public let voters: [ProposalVoter]
    public let proposerReturn: ProposerReturn?
    public let stats: ProposalStatsData
    public let myVote: ProposalMyVote?
    public let replyCount: Int
    public var id: String { summary.id }

    init(
        summary: ProposalSummary, voters: [ProposalVoter], proposerReturn: ProposerReturn? = nil,
        stats: ProposalStatsData? = nil, myVote: ProposalMyVote? = nil, replyCount: Int = 0
    ) {
        self.summary = summary
        self.voters = voters
        self.proposerReturn = proposerReturn
        self.stats = stats ?? ProposalStatsData(closesAt: summary.expiresAt)
        self.myVote = myVote
        self.replyCount = replyCount
    }

    init(_ value: Components.Schemas.ProposalDetail) {
        let proposal = Components.Schemas.Proposal(
            priceComparison: value.priceComparison, id: value.id, cabalId: value.cabalId, cabalName: value.cabalName,
            proposerId: value.proposerId, kind: value.kind,
            symbol: value.symbol, usdcMicros: value.usdcMicros, tokenAmount: value.tokenAmount,
            quoteOutAmount: value.quoteOutAmount, thesis: value.thesis, status: value.status,
            statusReason: value.statusReason, statusMessage: value.statusMessage, expiresAt: value.expiresAt,
            createdAt: value.createdAt, tally: value.tally,
            myBallot: value.myBallot.flatMap { Components.Schemas.Proposal.MyBallotPayload(rawValue: $0.rawValue) },
            canVote: value.canVote, network: value.network)
        self.init(
            summary: ProposalSummary(
                proposal, canVote: value.canVote, canWithdraw: value.canWithdraw, swap: ProposalSwap(value.swap)),
            voters: value.voters.map(ProposalVoter.init),
            proposerReturn: ProposerReturn(bps: value.proposerReturn1mBps, isPrivate: value.proposerReturnPrivate),
            stats: ProposalStatsData(value.stats), myVote: value.myVote.map(ProposalMyVote.init),
            replyCount: value.replyCount)
    }
}

public struct ProposalTally: Equatable, Sendable {
    public let yes: Int
    public let no: Int
    public let voters: Int
    public let needed: Int

    init(yes: Int, no: Int, voters: Int, needed: Int) {
        self.yes = yes
        self.no = no
        self.voters = voters
        self.needed = needed
    }

    init(_ value: Components.Schemas.Tally) {
        yes = value.yes
        no = value.no
        voters = value.voters
        needed = value.needed
    }
}

public struct ProposalVoteResult: Equatable, Sendable {
    public let status: ProposalStatus
    public let tally: ProposalTally
    public let undoUntil: Date?
    public let myVote: ProposalMyVote?

    init(
        status: ProposalStatus, tally: ProposalTally, undoUntil: Date? = nil, myVote: ProposalMyVote? = nil
    ) {
        self.status = status
        self.tally = tally
        self.undoUntil = undoUntil
        self.myVote = myVote
    }

    init(_ value: Components.Schemas.VoteResult) {
        self.init(
            status: ProposalStatus(value.status), tally: ProposalTally(value.tally), undoUntil: value.undoUntil,
            myVote: ProposalMyVote(value.myVote))
    }
}

public struct ProposalRetractResult: Equatable, Sendable {
    public let status: ProposalStatus
    public let tally: ProposalTally

    init(_ value: Components.Schemas.RetractVoteResult) {
        status = ProposalStatus(value.status)
        tally = ProposalTally(value.tally)
    }
}

public struct ProposalVoter: Equatable, Sendable, Identifiable {
    public let id: String
    public let ballot: String?

    init(_ value: Components.Schemas.ProposalVoter) {
        id = value.userId
        ballot = value.choice?.rawValue
    }
}

public struct ProposalMember: Equatable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let photoURL: URL?

    init(_ value: Components.Schemas.CabalMember) {
        id = value.userId
        name = value.displayName
        photoURL = value.photoUrl.flatMap(URL.init(string:))
    }
}

public struct ProposalAsset: Equatable, Sendable {
    public let displayName: String
    public let kind: AssetKind
    public let decimals: Int
    public let logoURL: URL?
    public let preIpo: PreIpoDetail?
    public let tradable: Bool

    var estimateMicros: Int64? { preIpo?.profile?.steps.last { $0.isEstimate }?.priceMicros }

    init(_ value: Components.Schemas.AssetDetail) {
        displayName = value.displayName
        kind = AssetKind(raw: value.kind.rawValue)
        decimals = value.decimals
        logoURL = value.logoUrl.flatMap(URL.init(string:))
        preIpo = PreIpoDetail(value)
        tradable = value.tradable
    }
}

public struct PendingVote: Identifiable, Equatable, Sendable {
    public let id: String
    public let cabalID: String
    public let cabalName: String
    public let kind: String
    public let symbol: String
    public let expiresAt: Date

    init(_ value: Components.Schemas.PendingVote) {
        id = value.proposalId
        cabalID = value.cabalId
        cabalName = value.cabalName
        kind = value.kind.rawValue
        symbol = value.symbol
        expiresAt = value.expiresAt
    }
}

public enum ProposalFilter: String, Sendable {
    case open, passed, executed, failed, closed, all

    func includes(_ status: ProposalStatus) -> Bool {
        switch self {
        case .all: true
        case .open: status == .open
        case .closed: status != .open
        case .passed: status == .passed
        case .executed: status == .executed
        case .failed: [.failed, .expired, .executionBlocked].contains(status)
        }
    }
}

public struct ProposalsRepository: Sendable {
    let api: APIClient

    public init(api: APIClient) { self.api = api }

    public func list(
        cabalID: String,
        filter: ProposalFilter,
        cursor: String?
    ) async throws -> (items: [ProposalSummary], nextCursor: String?) {
        try await api.read { client in
            let response = try await client.getCabalProposals(
                path: .init(id: cabalID),
                query: .init(filter: .init(rawValue: filter.rawValue), cursor: cursor)
            ).ok.body.json
            return (response.proposals.map { ProposalSummary($0) }, response.nextCursor)
        }
    }

    public func detail(id: String) async throws -> ProposalDetail {
        try await api.read { client in
            let value = try await client.getProposal(path: .init(id: id)).ok.body.json
            return ProposalDetail(value)
        }
    }

    public func isPaused(cabalID: String) async throws -> Bool {
        try await api.read { client in
            try await client.getCashOutPreview(path: .init(id: cabalID)).ok.body.json.pause != nil
        }
    }

    public func retrySwap(id: String, atCurrentPrice: Bool = false, submission: IdempotentSubmission) async throws {
        let body = atCurrentPrice ? Components.Schemas.SwapRetryRequest(atCurrentPrice: true) : nil
        try await api.submit(
            submission, payload: SwapRetryPayload(id: id, atCurrentPrice: atCurrentPrice), operation: "postSwapRetry"
        ) { client, key in
            _ = try await client.postSwapRetry(
                path: .init(id: id), headers: .init(idempotencyKey: key), body: body.map { .json($0) }
            ).accepted.body.json
        }
    }

    public func liveQuote(for proposal: ProposalSummary) async throws -> Int64? {
        let isSell = proposal.kind == "sell"
        return try await api.read { client in
            try await client.getCabalProposalPreview(
                path: .init(id: proposal.cabalID),
                query: .init(
                    kind: isSell ? .sell : .buy, symbol: proposal.symbol,
                    usdcMicros: isSell ? nil : proposal.usdcMicros, tokenAmount: isSell ? proposal.tokenAmount : nil)
            ).ok.body.json.quoteOutAmount
        }
    }

    public func priceComparison(cabalID: String, proposalID: String) async throws -> LivePriceComparison {
        try await api.read { client in
            let value = try await client.getCabalProposalPriceComparison(
                path: .init(id: cabalID, proposalId: proposalID)
            ).ok.body.json
            return LivePriceComparison(value)
        }
    }

    public func members(cabalID: String) async throws -> [ProposalMember] {
        try await api.read { client in
            try await client.getCabal(path: .init(id: cabalID)).ok.body.json.members.map(ProposalMember.init)
        }
    }

    public func asset(symbol: String) async throws -> ProposalAsset {
        try await api.read { client in
            ProposalAsset(try await client.getAsset(path: .init(symbol: symbol)).ok.body.json)
        }
    }

    @discardableResult
    public func vote(id: String, choice: String, submission: IdempotentSubmission) async throws -> ProposalVoteResult {
        guard let choice = Components.Schemas.BallotChoice(rawValue: choice) else {
            preconditionFailure("Unknown vote choice \(choice)")
        }
        let body = Components.Schemas.CastVoteRequest(choice: choice)
        return try await api.submit(submission, payload: body, operation: "postProposalVote") { client, key in
            ProposalVoteResult(
                try await client.postProposalVote(
                    path: .init(id: id),
                    headers: .init(idempotencyKey: key),
                    body: .json(body)
                ).ok.body.json)
        }
    }

    public func retractVote(id: String, submission: IdempotentSubmission) async throws -> ProposalRetractResult {
        try await api.submit(submission, payload: id, operation: "deleteProposalVote") { client, key in
            ProposalRetractResult(
                try await client.deleteProposalVote(
                    path: .init(id: id),
                    headers: .init(idempotencyKey: key)
                ).ok.body.json)
        }
    }

    func cardContext(symbols: Set<String>, cabalIDs: Set<String>) async -> ProposalCardContextPairs {
        async let assets = withTaskGroup(of: (String, ProposalAsset?).self) { group in
            for symbol in symbols {
                group.addTask { (symbol, try? await self.asset(symbol: symbol)) }
            }
            var pairs: [String: ProposalAsset] = [:]
            for await (symbol, asset) in group { if let asset { pairs[symbol] = asset } }
            return pairs
        }
        async let members = withTaskGroup(of: (String, [ProposalMember]?).self) { group in
            for id in cabalIDs {
                group.addTask { (id, try? await self.members(cabalID: id)) }
            }
            var pairs: [String: [ProposalMember]] = [:]
            for await (id, people) in group { if let people { pairs[id] = people } }
            return pairs
        }
        return await ProposalCardContextPairs(assets: assets, members: members)
    }

    public func closedProposals(cursor: String?) async throws -> (items: [ProposalSummary], nextCursor: String?) {
        try await api.read { client in
            let response = try await client.getMyProposals(query: .init(filter: .closed, cursor: cursor)).ok.body.json
            return (response.proposals.map { ProposalSummary($0) }, response.nextCursor)
        }
    }

    public func pendingVotes() async throws -> [PendingVote] {
        try await api.read { client in
            try await client.getMyPendingVotes().ok.body.json.map(PendingVote.init)
        }
    }
}

struct ProposalCardContextPairs: Sendable {
    let assets: [String: ProposalAsset]
    let members: [String: [ProposalMember]]
}

private struct SwapRetryPayload: Encodable, Sendable {
    let id: String
    let atCurrentPrice: Bool
}
