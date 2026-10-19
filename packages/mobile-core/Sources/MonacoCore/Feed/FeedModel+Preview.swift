import Foundation

public struct FeedFigures: Equatable, Sendable {
    public enum Side: Equatable, Sendable {
        case buy
        case sell
    }

    public enum AuthorReturn: Equatable, Sendable {
        case percent(String)
        case hidden
    }

    public var side: Side
    public var company: String?
    public var amount: String?
    public var detail: String?
    public var change: String?
    public var volume: String?
    public var likes: Int?
    public var authorReturn: AuthorReturn?

    public init(
        side: Side = .buy, company: String? = nil, amount: String? = nil, detail: String? = nil,
        change: String? = nil, volume: String? = nil, likes: Int? = nil, authorReturn: AuthorReturn? = nil
    ) {
        self.side = side
        self.company = company
        self.amount = amount
        self.detail = detail
        self.change = change
        self.volume = volume
        self.likes = likes
        self.authorReturn = authorReturn
    }
}

#if DEBUG
import HTTPTypes
import MonacoAPI
import OpenAPIRuntime

extension FeedModel {
    public enum PreviewMode: String, Sendable {
        case items
        case empty
        case failed
        case followsNobody
        case loadMoreFailed
        case pnlPrivate
        case loading
    }

    public static func preview(_ mode: PreviewMode, clock: any Clock<Duration>) -> FeedModel {
        FeedModel(
            api: previewClient(FeedPreviewTransport(mode: mode)),
            viewerID: mode == .followsNobody ? "01890a5d-ac96-774b-bcce-b302099a8058" : nil,
            hints: FeedPreviewHints(),
            clock: clock)
    }

    public static func previewFigures(_ mode: PreviewMode) -> [String: FeedFigures] {
        FeedPreviewFixtures.figures(hidingReturns: mode == .pnlPrivate)
    }
}

extension FeedItemDetailModel {
    public enum PreviewMode: String, Sendable {
        case ready
        case empty
        case replying
        case sending
        case failed
        case unavailable
    }

    public static let previewItemID = FeedPreviewFixtures.postItem.id

    public static func preview(_ mode: PreviewMode, clock: any Clock<Duration>) -> FeedItemDetailModel {
        FeedItemDetailModel(
            itemID: previewItemID, api: FeedModel.previewClient(FeedPostPreviewTransport(mode: mode)),
            hints: FeedPreviewHints(), clock: clock)
    }

    public static let previewCommentReturns = FeedPreviewFixtures.commentReturns
}

extension FeedModel {
    fileprivate static func previewClient(_ transport: any ClientTransport) -> APIClient {
        APIClient(
            serverURL: URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/"),
            tokens: FeedPreviewTokens(),
            transport: transport)
    }
}

enum FeedPreviewFixtures {
    static let cabalID = "00000000-0000-7000-8000-0000000fc002"
    static let newCabalID = "00000000-0000-7000-8000-0000000fc003"

    struct Entry {
        let item: Components.Schemas.FeedItem
        let figures: FeedFigures
    }

    static let entries: [Entry] = [
        entry(
            1, kind: "proposal", cabal: "Big Ballers", actor: 1, symbol: "NVDAx",
            title: "Mia proposed buying $400.00 of NVIDIA in Big Ballers", detail: "Voting open",
            body: "A small position alongside our existing holdings. "
                + "I want us to discuss concentration before adding more.",
            comments: 8, minutes: 12,
            figures: .init(
                company: "NVIDIA", amount: "$400.00", detail: "2.00% of Big Ballers’s pot", likes: 12,
                authorReturn: .percent("+8.2%"))),
        entry(
            2, kind: "trade", cabal: "Tech Crew", actor: 2, symbol: "AAPLx",
            title: "Tech Crew sold $480.00 of Apple", detail: "Filled at $240.00", comments: 3, minutes: 38,
            figures: .init(
                side: .sell, company: "Apple", amount: "$480.00", detail: "Sold 2 units · Avg $240.00", likes: 8,
                authorReturn: .percent("+6.7%"))),
        entry(
            3, kind: "price_move", cabal: nil, actor: nil, symbol: "NVDAx",
            title: "NVIDIA is up 4.18% today", detail: "$123.45, previous close $118.50", tone: "positive",
            comments: 4, minutes: 60,
            figures: .init(company: "NVIDIA", change: "+4.18%", volume: "$9.4M", likes: 18)),
        entry(
            4, kind: "reply", cabal: "Big Ballers", actor: 3, symbol: "AAPLx",
            title: "Priya replied to your proposal", detail: "AAPL · Replied to your proposal",
            body: "How much cash would we have left after this?", comments: 2, minutes: 70,
            figures: .init(likes: 5, authorReturn: .percent("+3.1%"))),
        entry(
            5, kind: "proposal", cabal: "Tech Crew", actor: 4, symbol: "MSFTx",
            title: "Sam proposed selling $350.00 of Microsoft in Tech Crew", detail: "Voting open",
            body: "Trim the position before earnings.", comments: 1, minutes: 95,
            figures: .init(
                side: .sell, company: "Microsoft", amount: "$350.00", detail: "1.40% of Tech Crew’s pot", likes: 3,
                authorReturn: .percent("−2.4%"))),
        entry(
            6, kind: "threshold", cabal: "Big Ballers", actor: 1, symbol: "MSFTx",
            title: "Mia’s proposal to buy $250.00 of Microsoft was approved", detail: "Execution pending",
            comments: 6, minutes: 130,
            figures: .init(
                company: "Microsoft", amount: "$250.00", detail: "1.00% of Big Ballers’s pot", likes: 6,
                authorReturn: .percent("+8.2%"))),
        entry(
            7, kind: "cabal_created", cabal: "Café Capital", cabalID: newCabalID, actor: 5, symbol: nil,
            title: "Jordan started Café Capital", detail: "Open to anyone", minutes: 60 * 5,
            figures: .init(likes: 2, authorReturn: .percent("+1.4%"))),
    ]

    static let olderEntries: [Entry] = [
        entry(
            8, kind: "trade", cabal: "Big Ballers", actor: 1, symbol: "GOOGLx",
            title: "Big Ballers bought $600.00 of Alphabet", detail: "Filled at $200.00", comments: 1,
            minutes: 60 * 26,
            figures: .init(
                company: "Alphabet", amount: "$600.00", detail: "Bought 3 units · Avg $200.00", likes: 4,
                authorReturn: .percent("+8.2%")))
    ]

    static var items: [Components.Schemas.FeedItem] { entries.map(\.item) }
    static var olderItems: [Components.Schemas.FeedItem] { olderEntries.map(\.item) }

    static func figures(hidingReturns: Bool) -> [String: FeedFigures] {
        Dictionary(
            uniqueKeysWithValues: (entries + olderEntries).map { entry in
                var figures = entry.figures
                if hidingReturns, entry.item.actorId != nil { figures.authorReturn = .hidden }
                return (entry.item.id, figures)
            })
    }

    static let postItem: Components.Schemas.FeedItem = {
        var item = entries[0].item
        item.title = "Proposed a buy"
        return item
    }()

    static let commentReturns: [String: FeedFigures.AuthorReturn] = [
        "author-mia": .percent("+8.2%"),
        "author-alex": .hidden,
    ]

    static let threads: [Components.Schemas.CommentThread] = [
        .init(
            comment: .sample(
                "c1", author: "Mia", body: "What would our NVIDIA allocation be after this?", minutesAgo: 12),
            replies: [
                .sample(
                    "c2", author: "Alex", parent: "c1", body: "Let’s review the current allocation before deciding.",
                    minutesAgo: 8)
            ])
    ]

    private static let actorNames = ["Mia", "Leo", "Priya", "Sam", "Jordan"]

    private static func entry(
        _ number: Int, kind: String, cabal: String?, cabalID: String = cabalID, actor: Int?, symbol: String?,
        title: String, detail: String?, body: String? = nil, tone: String = "neutral", comments: Int32 = 0,
        minutes: Double, figures: FeedFigures
    ) -> Entry {
        let created = Date(timeIntervalSinceNow: -minutes * 60)
        let item = Components.Schemas.FeedItem(
            id: String(format: "00000000-0000-7000-8000-0000000fe%03d", 100 + number), kind: kind, refType: kind,
            refId: String(format: "00000000-0000-7000-8000-0000000fa%03d", 100 + number),
            cabalId: cabal.map { _ in cabalID }, cabalName: cabal,
            actorId: actor.map { String(format: "00000000-0000-7000-8000-0000000fb%03d", $0) },
            actorName: actor.map { actorNames[$0 - 1] }, actorPhotoUrl: nil,
            assetId: symbol.map { _ in String(format: "00000000-0000-7000-8000-0000000fd%03d", number) },
            symbol: symbol, title: title, detail: detail, body: body, status: nil, tone: tone,
            commentCount: comments, createdAt: created, updatedAt: created)
        return Entry(item: item, figures: figures)
    }
}

private actor FeedPreviewPager {
    private var failedOnce = false

    func shouldFailNextPage() -> Bool {
        defer { failedOnce = true }
        return !failedOnce
    }
}

private struct FeedPreviewTransport: ClientTransport {
    let mode: FeedModel.PreviewMode
    private let pager = FeedPreviewPager()

    init(mode: FeedModel.PreviewMode) {
        self.mode = mode
    }

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        if mode == .failed { throw URLError(.notConnectedToInternet) }
        if mode == .loading { try await Task.sleep(for: .seconds(3600)) }
        if operationID == "getUser" { return try Self.reply(Self.profile) }
        let params = URLComponents(string: request.path ?? "")?.queryItems ?? []
        let value = { (name: String) in params.first { $0.name == name }?.value }
        if mode == .loadMoreFailed, value("cursor") != nil {
            if await pager.shouldFailNextPage() { throw URLError(.notConnectedToInternet) }
            return try Self.reply(Components.Schemas.FeedPage(items: FeedPreviewFixtures.olderItems, nextCursor: nil))
        }
        let kinds = value("kind").map { Set($0.split(separator: ",").map(String.init)) }
        let q = value("q")
        let items =
            mode == .empty || mode == .followsNobody
            ? []
            : FeedPreviewFixtures.items.filter { item in
                (kinds?.contains(item.kind) ?? true)
                    && (q.map { item.title.localizedCaseInsensitiveContains($0) } ?? true)
            }
        let more = mode == .loadMoreFailed ? "older" : nil
        return try Self.reply(Components.Schemas.FeedPage(items: items, nextCursor: more))
    }

    private static let profile = Components.Schemas.PublicProfile(
        id: "01890a5d-ac96-774b-bcce-b302099a8058", handle: "maya", displayName: "Maya Angelou",
        photoUrl: nil, followerCount: 0, followingCount: 0, followedByMe: false, blockedByMe: false)

    fileprivate static func reply(_ value: some Encodable) throws -> (HTTPResponse, HTTPBody?) {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(try encoder.encode(value)))
    }
}

private struct FeedPostPreviewTransport: ClientTransport {
    let mode: FeedItemDetailModel.PreviewMode

    func send(
        _: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        switch operationID {
        case "getFeedItem":
            if mode == .unavailable { return Self.notFound() }
            let detail = Components.Schemas.FeedItemDetail(
                item: FeedPreviewFixtures.postItem, visible: true, canComment: true)
            return try FeedPreviewTransport.reply(detail)
        case "getFeedComments":
            let threads = mode == .empty ? [] : FeedPreviewFixtures.threads
            return try FeedPreviewTransport.reply(Components.Schemas.CommentPage(items: threads, nextCursor: nil))
        case "postFeedComment":
            if mode == .sending { try await Task.sleep(for: .seconds(3600)) }
            if mode == .failed { throw URLError(.notConnectedToInternet) }
            var created = try FeedPreviewTransport.reply(Components.Schemas.Comment.sample("c9", mine: true))
            created.0.status = .created
            return created
        default:
            throw URLError(.unsupportedURL)
        }
    }

    private static func notFound() -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: .notFound)
        response.headerFields[.contentType] = "application/problem+json"
        let problem =
            #"{"type":"about:blank","title":"Not Found","status":404,"code":"feed_item_not_found","#
            + #""message":"That update isn’t available.","trace_id":"00000000000000000000000000000000","retryable":false}"#
        return (response, HTTPBody(problem))
    }
}

private struct FeedPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct FeedPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
