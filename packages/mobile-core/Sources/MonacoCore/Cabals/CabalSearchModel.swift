import Foundation
import MonacoAPI
import Observation

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public struct CabalSearchRow: Identifiable, Equatable, Sendable {
    public enum Action: Equatable, Sendable {
        case request
        case requested
        case member
    }

    public let id: String
    public let name: String
    public let pictureURL: String?
    public let detail: String
    public let joinPolicy: CabalJoinPolicy
    public var action: Action

    init(_ item: Components.Schemas.CabalSearchItem) {
        id = item.id
        name = item.name
        pictureURL = item.pictureUrl
        joinPolicy = CabalJoinPolicy(wire: item.joinMode)
        detail = "\(CabalCopy.memberCount(item.memberCount)) · \(joinPolicy.label)"
        action = Self.action(item)
    }

    private static func action(_ item: Components.Schemas.CabalSearchItem) -> Action {
        if item.isMember { return .member }
        if item.myAccessRequestStatus == "pending" { return .requested }
        return .request
    }
}

public enum CabalSearchState: Equatable, Sendable {
    case idle
    case loading
    case empty(String)
    case failed(APIError)
    case rows([CabalSearchRow])
}

@Observable
@MainActor
public final class CabalSearchModel {
    public static let pageSize = 20

    public var query = ""
    public private(set) var pager: CursorPager<CabalSearchRow>?
    public private(set) var searched = ""
    public private(set) var entering: Set<String> = []
    public private(set) var toast: CabalInviteToast?

    private let api: APIClient
    private let hints: any HintSource
    private let refresher: HintRefresher
    private var entered: [String: CabalSearchRow.Action] = [:]
    private var submissions: [String: IdempotentSubmission] = [:]
    private var toastSerial = 0

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
        let hook = SearchReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in
            await self?.refresh()
        }
    }

    public var trimmedQuery: String {
        query.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public var isActive: Bool { !trimmedQuery.isEmpty }

    public var state: CabalSearchState {
        guard isActive, let pager, searched == trimmedQuery else { return isActive ? .loading : .idle }
        if !pager.items.isEmpty {
            return .rows(pager.items.map(overlaid))
        }
        switch pager.phase {
        case .idle, .loadingFirst, .loadingMore: return .loading
        case .exhausted: return .empty(searched)
        case .failed(let error): return .failed(error)
        }
    }

    private func overlaid(_ row: CabalSearchRow) -> CabalSearchRow {
        var row = row
        if let action = entered[row.id] { row.action = action }
        return row
    }

    public var canLoadMore: Bool { pager?.phase == .idle }

    public func search() async {
        let text = trimmedQuery
        guard !text.isEmpty else {
            pager = nil
            searched = ""
            return
        }
        let api = api
        let pager = CursorPager<CabalSearchRow> { cursor in
            let page = try await api.read { client in
                try await client.getCabals(
                    query: .init(query: text, cursor: cursor, limit: Self.pageSize)
                ).ok.body.json
            }
            return (page.items.map(CabalSearchRow.init), page.nextCursor)
        }
        self.pager = pager
        searched = text
        entered = [:]
        await pager.loadFirst()
    }

    public func loadMore() async {
        guard let pager else { return }
        await pager.loadMore()
        toastFailureOverRows()
    }

    public func refresh() async {
        guard let pager else { return }
        await pager.refreshFirstPage()
        toastFailureOverRows()
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .user(what: "cabal_access")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func enter(_ row: CabalSearchRow) async -> String? {
        guard !entering.contains(row.id), row.action == .request else { return nil }
        entering.insert(row.id)
        defer { entering.remove(row.id) }
        let submission = submissions[row.id] ?? IdempotentSubmission()
        submissions[row.id] = submission
        switch await api.enterCabal(row.id, policy: row.joinPolicy, submission: submission) {
        case .joined:
            entered[row.id] = .member
            show(CabalEntry.joinedToast, success: true)
            return row.id
        case .alreadyMember:
            entered[row.id] = .member
            return row.id
        case .requested:
            entered[row.id] = .requested
            show(CabalEntry.requestedToast, success: true)
            return nil
        case .requestPending:
            entered[row.id] = .requested
            return nil
        case .refused(let error):
            show(ToastCopy.message(for: error), success: false)
            return nil
        }
    }

    private func toastFailureOverRows() {
        guard let pager, case .failed(let error) = pager.phase, !pager.items.isEmpty else { return }
        show(ToastCopy.message(for: error), success: false)
    }

    private func show(_ message: String, success: Bool) {
        toastSerial += 1
        toast = CabalInviteToast(serial: toastSerial, message: message, isSuccess: success)
    }
}

private final class SearchReloadHook {
    var run: (@MainActor () async -> Void)?
}

#if DEBUG
extension CabalSearchModel {
    public static func preview(
        _ items: [Components.Schemas.CabalSearchItem] = Components.Schemas.CabalSearchItem.samples
    ) -> CabalSearchModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        return CabalSearchModel(
            api: APIClient(
                serverURL: serverURL, tokens: CabalSearchPreviewTokens(),
                transport: CabalSearchPreviewTransport(items: items)
            ),
            hints: CabalSearchPreviewHints()
        )
    }
}

private struct CabalSearchPreviewTransport: ClientTransport {
    let items: [Components.Schemas.CabalSearchItem]

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        switch operationID {
        case "getCabals":
            let query = URLComponents(string: request.path ?? "")?.queryItems?.first { $0.name == "query" }?.value ?? ""
            let matches = items.filter { $0.name.localizedCaseInsensitiveContains(query) }
            let page = Components.Schemas.CabalSearchPage(items: matches, nextCursor: nil)
            return (response, HTTPBody(try encoder.encode(page)))
        case "postCabalAccessRequest":
            response.status = .created
            return (response, HTTPBody(#"{"id":"preview","direction":"request","status":"pending"}"#))
        default:
            return (response, HTTPBody(try encoder.encode(Components.Schemas.Cabal.sample(role: "member"))))
        }
    }
}

private struct CabalSearchPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct CabalSearchPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
