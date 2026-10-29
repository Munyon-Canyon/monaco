import Foundation

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

    static let items: [Components.Schemas.FeedItem] = [
        item(
            1, kind: "proposal", cabal: "Big Ballers", actor: 1, symbol: "NVDAx",
            title: "Mia proposed buying $400.00 of NVIDIA in Big Ballers", detail: "Voting open",
            body: "A small position alongside our existing holdings. "
                + "I want us to discuss concentration before adding more.",
            comments: 8, minutes: 12, likes: 12, returnBps: 820,
            figures: .init(action: "buy", usdcMicros: 400_000_000, assetName: "NVIDIA", shareOfPotBps: 200)),
        Components.Schemas.FeedItem.results[0],
        Components.Schemas.FeedItem.results[1],
        item(
            4, kind: "reply", cabal: "Tech Titans", actor: 6, symbol: "TSLAx",
            title: "Logan replied to Sell TSLA", detail: "TSLA · Replied to Sell TSLA",
            body: "Taking profit here is the right call before the deliveries print.", comments: 2, minutes: 70,
            likes: 5, returnBps: 310, figures: .init()),
        item(
            5, kind: "member_joined", cabal: "Big Ballers", actor: 7, symbol: nil,
            title: "Nora joined Big Ballers", detail: nil, minutes: 95, likes: 2, figures: .init()),
        Components.Schemas.FeedItem.results[3],
        Components.Schemas.FeedItem.results[2],
        item(
            8, kind: "trade", cabal: "Big Ballers", actor: 3, symbol: "AMZNx",
            title: "Priya sold $320.00 of Amazon in Big Ballers", detail: "Filled at $160.00", comments: 1,
            minutes: 60 * 5, likes: 3, returnBps: 140,
            figures: .init(
                action: "sell", usdcMicros: 320_000_000, assetName: "Amazon", tokenAmount: 200_000_000,
                tokenDecimals: 8, priceMicros: 160_000_000)),
    ]

    static let olderItems: [Components.Schemas.FeedItem] = [
        item(
            9, kind: "trade", cabal: "Big Ballers", actor: 1, symbol: "GOOGLx",
            title: "Big Ballers bought $600.00 of Alphabet", detail: "Filled at $200.00", comments: 1,
            minutes: 60 * 26, likes: 4, returnBps: 820,
            figures: .init(
                action: "buy", usdcMicros: 600_000_000, assetName: "Alphabet", tokenAmount: 300_000_000,
                tokenDecimals: 8, priceMicros: 200_000_000))
    ]

    static func hidingReturns(_ items: [Components.Schemas.FeedItem]) -> [Components.Schemas.FeedItem] {
        items.map { item in
            var item = item
            if item.actorId != nil {
                item.return1mBps = nil
                item.returnPrivate = true
            }
            return item
        }
    }

    static let postItem: Components.Schemas.FeedItem = {
        var item = items[0]
        item.title = "Proposed a buy"
        return item
    }()

    static let threads: [Components.Schemas.CommentThread] = [
        .init(
            comment: .sample(
                "c1", author: "Mia", body: "What would our NVIDIA allocation be after this?", minutesAgo: 12
            ).returning(820),
            replies: [
                .sample(
                    "c2", author: "Alex", parent: "c1", body: "Let’s review the current allocation before deciding.",
                    minutesAgo: 8
                ).returning(nil, isPrivate: true)
            ])
    ]

    private static let actorNames = ["Mia", "Leo", "Priya", "Sam", "Jordan", "Logan", "Nora"]

    struct Figures {
        var action: String?
        var usdcMicros: Int64?
        var assetName: String?
        var tokenAmount: Int64?
        var tokenDecimals: Int?
        var priceMicros: Int64?
        var shareOfPotBps: Int64?
        var changeBps: Int64?
        var markMicros: Int64?
        var prevCloseMicros: Int64?
    }

    private static func item(
        _ number: Int, kind: String, cabal: String?, cabalID: String = cabalID, actor: Int?, symbol: String?,
        title: String, detail: String?, body: String? = nil, tone: String = "neutral", comments: Int32 = 0,
        minutes: Double, likes: Int32, returnBps: Int64? = nil, figures: Figures
    ) -> Components.Schemas.FeedItem {
        let created = Date(timeIntervalSinceNow: -minutes * 60)
        let proposal = ["proposal", "threshold", "reply"].contains(kind)
        return Components.Schemas.FeedItem(
            id: String(format: "00000000-0000-7000-8000-0000000fe%03d", 100 + number), kind: kind, refType: kind,
            refId: String(format: "00000000-0000-7000-8000-0000000fa%03d", 100 + number),
            cabalId: cabal.map { _ in cabalID }, cabalName: cabal,
            actorId: actor.map { String(format: "00000000-0000-7000-8000-0000000fb%03d", $0) },
            actorName: actor.map { actorNames[$0 - 1] }, actorPhotoUrl: nil,
            assetId: symbol.map { _ in String(format: "00000000-0000-7000-8000-0000000fd%03d", number) },
            symbol: symbol, title: title, detail: detail, body: body, status: nil, tone: tone,
            action: figures.action, usdcMicros: figures.usdcMicros, assetName: figures.assetName,
            tokenAmount: figures.tokenAmount, tokenDecimals: figures.tokenDecimals, priceMicros: figures.priceMicros,
            shareOfPotBps: figures.shareOfPotBps, changeBps: figures.changeBps, markMicros: figures.markMicros,
            prevCloseMicros: figures.prevCloseMicros,
            proposalId: proposal ? String(format: "00000000-0000-7000-8000-0000000fa%03d", 100 + number) : nil,
            commentCount: comments,
            threadItemId: String(format: "00000000-0000-7000-8000-0000000fe%03d", number),
            replyCount: comments, canReply: true, canReact: true, replyRule: .anyone, replyRuleCabalName: nil,
            likeCount: likes, likedByMe: false, reactions: [], reactionCount: 0,
            return1mBps: returnBps, returnPrivate: false,
            createdAt: created, updatedAt: created)
    }
}

extension Components.Schemas.Comment {
    fileprivate func returning(_ bps: Int64?, isPrivate: Bool = false) -> Self {
        var comment = self
        comment.author.return1mBps = bps
        comment.author.returnPrivate = isPrivate
        return comment
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
        if operationID == "putFeedLike" || operationID == "deleteFeedLike" {
            return (HTTPResponse(status: .noContent), nil)
        }
        let params = URLComponents(string: request.path ?? "")?.queryItems ?? []
        let value = { (name: String) in params.first { $0.name == name }?.value }
        if mode == .loadMoreFailed, value("cursor") != nil {
            if await pager.shouldFailNextPage() { throw URLError(.notConnectedToInternet) }
            return try Self.reply(Components.Schemas.FeedPage(items: FeedPreviewFixtures.olderItems, nextCursor: nil))
        }
        let kinds = value("kind").map { Set($0.split(separator: ",").map(String.init)) }
        let shown =
            mode == .pnlPrivate
            ? FeedPreviewFixtures.hidingReturns(FeedPreviewFixtures.items) : FeedPreviewFixtures.items
        let items =
            mode == .empty || mode == .followsNobody
            ? []
            : shown.filter { kinds?.contains($0.kind) ?? true }
        let more = mode == .loadMoreFailed ? "older" : nil
        return try Self.reply(Components.Schemas.FeedPage(items: items, nextCursor: more))
    }

    private static let profile = Components.Schemas.PublicProfile(
        id: "01890a5d-ac96-774b-bcce-b302099a8058", handle: "maya", displayName: "Maya Angelou",
        photoUrl: nil, bio: nil, createdAt: Date(timeIntervalSince1970: 1_759_233_600), cabalCount: 0,
        followerCount: 0, followingCount: 0, followedByMe: false, blockedByMe: false,
        returnPeriod: ._1m, returnPrivate: false, crown: false, cabalsInCommon: 0)

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
