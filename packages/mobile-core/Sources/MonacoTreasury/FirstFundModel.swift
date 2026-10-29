import Foundation
import MonacoAPI
import Observation

public struct FirstFundVote: Equatable, Sendable {
    public let proposalID: String
    public let kind: String
    public let symbol: String
    public let assetName: String
    public let logoURL: String?
    public let usdcMicros: Int64?
    public let tokenAmount: Int64?
    public let yes: Int
    public let no: Int
    public let voters: Int
    public let needed: Int
    public let replies: Int
    public let expiresAt: Date

    public init(
        proposalID: String, kind: String, symbol: String, assetName: String, logoURL: String?, usdcMicros: Int64?,
        tokenAmount: Int64?, yes: Int, no: Int, voters: Int, needed: Int, replies: Int, expiresAt: Date
    ) {
        self.proposalID = proposalID
        self.kind = kind
        self.symbol = symbol
        self.assetName = assetName
        self.logoURL = logoURL
        self.usdcMicros = usdcMicros
        self.tokenAmount = tokenAmount
        self.yes = yes
        self.no = no
        self.voters = voters
        self.needed = needed
        self.replies = replies
        self.expiresAt = expiresAt
    }

    init(_ vote: Components.Schemas.PendingVote) {
        proposalID = vote.proposalId
        kind = vote.kind.rawValue
        symbol = vote.symbol ?? ""
        assetName = vote.assetName ?? ""
        logoURL = vote.assetLogoUrl
        usdcMicros = vote.usdcMicros
        tokenAmount = vote.tokenAmount
        yes = Int(vote.tally.yes)
        no = Int(vote.tally.no)
        voters = Int(vote.tally.voters)
        needed = Int(vote.tally.needed)
        replies = vote.replyCount
        expiresAt = vote.expiresAt
    }
}

public struct FirstFundMember: Equatable, Identifiable, Sendable {
    public let id: String
    public let name: String
    public let photoURL: String?
    public let crown: Bool

    public init(id: String, name: String, photoURL: String?, crown: Bool) {
        self.id = id
        self.name = name
        self.photoURL = photoURL
        self.crown = crown
    }

    init(_ row: Components.Schemas.CabalMemberRow) {
        id = row.userId
        name = row.displayName.isEmpty ? (row.handle ?? "") : row.displayName
        photoURL = row.photoUrl
        crown = row.crown
    }
}

public struct FirstFundValue: Equatable, Sendable {
    public let cabal: Components.Schemas.Cabal
    public let settledMicros: Int64
    public let shareBps: Int32?
    public let potValueMicros: Int64?
    public let vote: FirstFundVote?
    public let members: [FirstFundMember]

    public init(
        cabal: Components.Schemas.Cabal, settledMicros: Int64, shareBps: Int32?, potValueMicros: Int64?,
        vote: FirstFundVote?, members: [FirstFundMember]
    ) {
        self.cabal = cabal
        self.settledMicros = settledMicros
        self.shareBps = shareBps
        self.potValueMicros = potValueMicros
        self.vote = vote
        self.members = members
    }

    public var memberCount: Int { Int(cabal.memberCount) }

    nonisolated public static func shareText(bps: Int32) -> String {
        let value = Int(max(bps, 0))
        let whole = value / 100
        let cents = value % 100
        if cents == 0 { return "\(whole)%" }
        if cents % 10 == 0 { return "\(whole).\(cents / 10)%" }
        return "\(whole).\(cents < 10 ? "0" : "")\(cents)%"
    }

    public var shareText: String? { shareBps.map { FirstFundValue.shareText(bps: $0) } }
}

@Observable
@MainActor
public final class FirstFundModel {
    nonisolated public static let stripLimit = 12

    public private(set) var state: LoadState<FirstFundValue> = .idle

    public let cabalID: String

    @ObservationIgnored private let api: APIClient
    @ObservationIgnored private let viewerID: String
    @ObservationIgnored private let settledMicros: Int64

    public init(api: APIClient, cabalID: String, viewerID: String, settledMicros: Int64) {
        self.api = api
        self.cabalID = cabalID
        self.viewerID = viewerID
        self.settledMicros = settledMicros
    }

    public var value: FirstFundValue? {
        if case .loaded(let value) = state { return value }
        return nil
    }

    public func load() async {
        if value == nil { state = .loading }
        let id = cabalID
        async let pot = try? await api.read { try await $0.getCabalPot(path: .init(id: id)).ok.body.json }
        async let cabal = Self.result {
            try await self.api.read { try await $0.getCabal(path: .init(id: id)).ok.body.json }
        }
        async let votes = try? await api.read { try await $0.getMyPendingVotes().ok.body.json }
        async let page = try? await api.read {
            try await $0.getCabalMembers(path: .init(id: id), query: .init(limit: Self.stripLimit, order: .roleShare))
                .ok.body.json
        }
        let (potRead, cabalResult, voteRead, pageRead) = await (pot, cabal, votes, page)
        let cabalRead: Components.Schemas.Cabal
        switch cabalResult {
        case .success(let read): cabalRead = read
        case .failure(let error):
            if value == nil { state = .failed(APIError(error)) }
            return
        }
        state = .loaded(
            FirstFundValue(
                cabal: cabalRead, settledMicros: settledMicros, shareBps: potRead?.me?.sliceBps,
                potValueMicros: potRead?.potValueMicros,
                vote: voteRead?.first { $0.cabalId == id }.map(FirstFundVote.init),
                members: Self.strip(pageRead?.items ?? [], viewerID: viewerID)))
    }

    private static func result<T: Sendable>(_ work: @Sendable () async throws -> T) async -> Result<T, any Error> {
        do { return .success(try await work()) } catch { return .failure(error) }
    }

    nonisolated public static func strip(
        _ rows: [Components.Schemas.CabalMemberRow], viewerID: String
    ) -> [FirstFundMember] {
        let others = rows.filter { $0.userId != viewerID }
        let ordered = others.filter(\.crown) + others.filter { !$0.crown }
        return ordered.prefix(stripLimit).map(FirstFundMember.init)
    }

    nonisolated public static func heldNoShare(sliceBps: Int32?) -> Bool {
        (sliceBps ?? 0) == 0
    }
}
