import MonacoAPI
import Observation

public struct CabalMemberRow: Identifiable, Equatable, Sendable {
    public let member: Components.Schemas.CabalMember
    public var id: String { member.userId }

    public init(_ member: Components.Schemas.CabalMember) {
        self.member = member
    }

    public init(_ row: Components.Schemas.CabalMemberRow) {
        member = .init(
            userId: row.userId, handle: row.handle, displayName: row.displayName, photoUrl: row.photoUrl,
            role: row.role.rawValue, canVote: row.canVote, joinedAt: row.joinedAt)
    }
}

@Observable
@MainActor
public final class CabalMembersPager {
    public static let pageSize = 50

    private let pager: CursorPager<CabalMemberRow>

    public init(cabalID: String, api: APIClient, pageSize: Int = CabalMembersPager.pageSize) {
        pager = CursorPager<CabalMemberRow> { cursor in
            let page = try await api.read { client in
                try await client.getCabalMembers(
                    path: .init(id: cabalID), query: .init(cursor: cursor, limit: pageSize)
                ).ok.body.json
            }
            return (page.items.map { CabalMemberRow($0) }, page.nextCursor)
        }
    }

    init(fetch: @escaping @Sendable (String?) async throws -> (items: [CabalMemberRow], nextCursor: String?)) {
        pager = CursorPager<CabalMemberRow>(fetch: fetch)
    }

    public var rows: [CabalMemberRow] { pager.items }

    public var phase: PagerPhase { pager.phase }

    public var hasMore: Bool { pager.phase == .idle || pager.phase == .loadingMore }

    public func loadFirst() async {
        await pager.loadFirst()
    }

    public func loadMore() async {
        await pager.loadMore()
    }

    public func loadMore(after row: CabalMemberRow) async {
        guard row.id == pager.items.last?.id else { return }
        await pager.loadMore()
    }
}
