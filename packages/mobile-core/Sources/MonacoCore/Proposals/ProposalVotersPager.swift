import Foundation
import MonacoAPI
import Observation

public struct ProposalVoterRow: Identifiable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let photoURL: URL?
    public let voted: Bool
    public let ballot: String?
    public let castAt: Date?

    public var voter: ProposalVoter {
        ProposalVoter(id: id, ballot: ballot, voted: voted, castAt: castAt, displayName: name, photoURL: photoURL)
    }

    init(_ value: Components.Schemas.ProposalVoteItem) {
        id = value.userId
        name = value.displayName
        photoURL = value.photoUrl.flatMap(URL.init(string:))
        voted = value.voted
        ballot = value.choice?.rawValue
        castAt = value.castAt
    }
}

public struct ProposalVoterTotals: Equatable, Sendable {
    public let yes: Int
    public let no: Int
    public let notVoted: Int

    public init(_ tally: ProposalTally) {
        yes = tally.yes
        no = tally.no
        notVoted = max(0, tally.voters - tally.yes - tally.no)
    }

    public var voted: Int { yes + no }

    public func counts(sidesVisible: Bool) -> [String] {
        sidesVisible
            ? ["\(yes) yes", "\(no) no", "\(notVoted) not voted"] : ["\(voted) voted", "\(notVoted) not voted"]
    }

    public func strip(sidesVisible: Bool) -> String {
        counts(sidesVisible: sidesVisible).joined(separator: " · ")
    }
}

@Observable
@MainActor
public final class ProposalVotersPager {
    public static let pageSize = 50

    private let pager: CursorPager<ProposalVoterRow>

    public init(proposalID: String, api: APIClient, pageSize: Int = ProposalVotersPager.pageSize) {
        pager = CursorPager<ProposalVoterRow> { cursor in
            let page = try await api.read { client in
                try await client.getProposalVotes(
                    path: .init(id: proposalID), query: .init(cursor: cursor, limit: pageSize)
                ).ok.body.json
            }
            return (page.items.map(ProposalVoterRow.init), page.nextCursor)
        }
    }

    init(fetch: @escaping @Sendable (String?) async throws -> (items: [ProposalVoterRow], nextCursor: String?)) {
        pager = CursorPager<ProposalVoterRow>(fetch: fetch)
    }

    public var rows: [ProposalVoterRow] { pager.items }

    public var phase: PagerPhase { pager.phase }

    public func loadFirst() async {
        await pager.loadFirst()
    }

    public func loadMore() async {
        await pager.loadMore()
    }

    public func loadMore(after row: ProposalVoterRow) async {
        guard row.id == pager.items.last?.id else { return }
        await pager.loadMore()
    }
}
