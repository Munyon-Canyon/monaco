import Foundation
import MonacoAPI
import Observation

public struct CabalMemberEntry: Identifiable, Equatable, Sendable {
    public let row: Components.Schemas.CabalMemberRow
    public var id: String { row.userId }

    public init(_ row: Components.Schemas.CabalMemberRow) {
        self.row = row
    }
}

@Observable
@MainActor
public final class CabalMembersModel {
    public static let pageSize = 50
    public static let previewCount = 5

    public var totals: Components.Schemas.CabalMemberTotals? { totalsBox.value }
    public private(set) var toast: CabalInviteToast?
    public private(set) var deciding: Set<String> = []

    public let cabal: Components.Schemas.Cabal
    private let api: APIClient
    private let hints: any HintSource
    private let access: CabalAccessModel
    private let pager: CursorPager<CabalMemberEntry>
    private let refresher: HintRefresher
    private let totalsBox: TotalsBox
    private var decisions: [String: IdempotentSubmission] = [:]
    private var toastSerial = 0

    public init(
        cabal: Components.Schemas.Cabal, api: APIClient, hints: any HintSource,
        pageSize: Int = CabalMembersModel.pageSize
    ) {
        self.cabal = cabal
        self.api = api
        self.hints = hints
        access = CabalAccessModel(cabalID: cabal.id, api: api, hints: hints)
        let box = TotalsBox()
        totalsBox = box
        let cabalID = cabal.id
        pager = CursorPager<CabalMemberEntry> { cursor in
            let page = try await api.read { client in
                try await client.getCabalMembers(
                    path: .init(id: cabalID), query: .init(cursor: cursor, limit: pageSize, order: .roleShare)
                ).ok.body.json
            }
            await box.set(page.totals)
            return (page.items.map(CabalMemberEntry.init), page.nextCursor)
        }
        let hook = MembersReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in await self?.reload() }
    }

    public var rows: [CabalMemberEntry] { pager.items }

    public var phase: PagerPhase { pager.phase }

    public var hasMore: Bool { pager.phase == .idle || pager.phase == .loadingMore }

    public var isAdmin: Bool { cabal.me?.role == "creator" || cabal.me?.role == "admin" }

    public var canPropose: Bool { cabal.me?.canPropose == true }

    public var requests: [CabalPendingRequest] {
        if case .pending(let list) = access.standing { return list }
        return []
    }

    public var requestsFailed: Bool { access.loadFailed }

    public var preview: [CabalMemberEntry] { Array(rows.prefix(Self.previewCount)) }

    public func rows(in group: CabalMemberGroup) -> [CabalMemberEntry] {
        rows.filter { CabalMemberGroup($0.row) == group }
    }

    public var voting: [CabalMemberEntry] { rows.filter { CabalMemberGroup($0.row).votes } }

    public var notVoting: [CabalMemberEntry] { rows.filter { !CabalMemberGroup($0.row).votes } }

    public func loadFirst() async {
        async let members: Void = pager.loadFirst()
        async let pending: Void = loadRequests()
        await members
        await pending
    }

    public func loadMore(after entry: CabalMemberEntry) async {
        guard entry.id == rows.last?.id else { return }
        await pager.loadMore()
    }

    public func retryMore() async {
        await pager.loadMore()
    }

    public func approve(_ requestID: String) async {
        await decide(requestID, approve: true)
    }

    public func deny(_ requestID: String) async {
        await decide(requestID, approve: false)
    }

    public func observe() async {
        await refresher.observe([
            hints.hints(matching: .cabal(id: cabal.id, what: "members")),
            hints.hints(matching: .cabal(id: cabal.id, what: "access_requests")),
        ])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    private func loadRequests() async {
        guard isAdmin else { return }
        await access.load(cabal: cabal)
    }

    private func reload() async {
        async let members: Void = pager.refreshFirstPage()
        async let pending: Void = loadRequests()
        await members
        await pending
    }

    private func decide(_ requestID: String, approve: Bool) async {
        guard isAdmin, !deciding.contains(requestID),
            let request = requests.first(where: { $0.id == requestID })
        else { return }
        deciding.insert(requestID)
        defer { deciding.remove(requestID) }
        let submission = decisions[requestID] ?? IdempotentSubmission()
        decisions[requestID] = submission
        do {
            try await api.decideAccessRequest(
                cabalID: cabal.id, requestID: requestID, approve: approve, submission: submission)
            if approve {
                show(CabalMemberCopy.joinedToast(name: request.name, cabal: cabal.name), success: true)
            } else {
                show(CabalMemberCopy.declinedToast, success: true)
            }
        } catch {
            show(ToastCopy.message(for: APIError(error)), success: false)
        }
        await reload()
    }

    private func show(_ message: String, success: Bool) {
        toastSerial += 1
        toast = CabalInviteToast(serial: toastSerial, message: message, isSuccess: success)
    }
}

@Observable
@MainActor
private final class TotalsBox {
    private(set) var value: Components.Schemas.CabalMemberTotals?

    func set(_ totals: Components.Schemas.CabalMemberTotals) {
        value = totals
    }
}

private final class MembersReloadHook {
    var run: (@MainActor () async -> Void)?
}
