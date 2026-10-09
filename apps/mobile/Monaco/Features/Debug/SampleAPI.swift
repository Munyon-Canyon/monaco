#if DEBUG
import Foundation
import MonacoAPI
import Synchronization

nonisolated struct SampleAPIScript: Sendable {
    enum Mode: Sendable {
        case populated
        case empty
        case hang
        case balanceFails
        case cabalsUnavailable
        case assetsUnavailable
        case chartUnavailable
        case potUnavailable
    }

    var mode = Mode.populated
    var role = "creator"
    var pictureURL: String?
    var pictureWriteFails = false
    var balance = Components.Schemas.Balance.sample
    var asset = Components.Schemas.AssetDetail.googl
    var chart = Components.Schemas.AssetChart.oneDay
    var cabalCount = 1
    var pot = Components.Schemas.CabalPot.sampleInvested
}

nonisolated final class SampleAPIProtocol: URLProtocol {
    private static let script = Mutex(SampleAPIScript())
    private static let registered = Mutex(false)
    private static let createdName = Mutex<String?>(nil)

    static func install(_ next: SampleAPIScript) {
        script.withLock { $0 = next }
        let needsRegistering = registered.withLock { done in
            defer { done = true }
            return !done
        }
        if needsRegistering { URLProtocol.registerClass(SampleAPIProtocol.self) }
    }

    override class func canInit(with request: URLRequest) -> Bool {
        request.url?.path.hasPrefix("/v1/") == true && request.url?.host != "127.0.0.1"
    }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func stopLoading() {}

    override func startLoading() {
        let script = Self.script.withLock { $0 }
        guard script.mode != .hang, let url = request.url else { return }
        let method = request.httpMethod ?? "GET"
        if method == "POST", url.path == "/v1/cabals" { Self.createdName.withLock { $0 = requestedName() } }
        let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems ?? []
        let query = Dictionary(items.compactMap { item in item.value.map { (item.name, $0) } }) { first, _ in first }
        let reply = Self.reply(path: url.path, method: method, query: query, script: script)
        let response = HTTPURLResponse(
            url: url, statusCode: reply.status, httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": reply.contentType]
        )
        guard let response else { return }
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: reply.body)
        client?.urlProtocolDidFinishLoading(self)
    }

    private func requestedName() -> String? {
        var body = request.httpBody
        if body == nil, let stream = request.httpBodyStream {
            stream.open()
            defer { stream.close() }
            var collected = Data()
            var buffer = [UInt8](repeating: 0, count: 4_096)
            while stream.hasBytesAvailable {
                let count = stream.read(&buffer, maxLength: buffer.count)
                if count <= 0 { break }
                collected.append(buffer, count: count)
            }
            body = collected
        }
        guard let body, let object = try? JSONSerialization.jsonObject(with: body) as? [String: Any] else { return nil }
        return object["name"] as? String
    }

    private struct Reply {
        var status = 200
        var contentType = "application/json"
        var body: Data
    }

    private static func json(_ value: some Encodable) -> Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return Reply(body: (try? encoder.encode(value)) ?? Data("{}".utf8))
    }

    private static func raw(_ text: String) -> Reply { Reply(body: Data(text.utf8)) }

    private static func problem(_ status: Int, _ code: String, _ message: String) -> Reply {
        Reply(
            status: status, contentType: "application/problem+json",
            body: Data(
                #"{"type":"about:blank","title":"\#(message)","status":\#(status),"code":"\#(code)","detail":"\#(message)","retryable":false}"#
                    .utf8))
    }

    private static func reply(path: String, method: String, query: [String: String], script: SampleAPIScript) -> Reply {
        let range = query["range"]
        let empty = script.mode == .empty
        if script.mode == .cabalsUnavailable,
            ["/v1/me/cabals", "/v1/me/portfolio", "/v1/leaderboards/cabals"].contains(path)
        {
            return problem(503, "unavailable", "Cabals can't be read right now.")
        }
        if method == "POST", path == "/v1/cabals" {
            var created = Components.Schemas.Cabal.sample(role: "creator")
            created.id = CabalsTabSampleData.createdCabalID
            created.name = createdName.withLock { $0 } ?? created.name
            var reply = json(created)
            reply.status = 201
            return reply
        }
        let parts = path.split(separator: "/").map(String.init)
        if method == "PUT" || method == "DELETE", parts.last == "picture", script.pictureWriteFails {
            return problem(413, "picture_invalid", "Picture must be at most 2MB.")
        }
        if parts.count >= 3, parts[1] == "cabals", UUID(uuidString: parts[2]) != nil || parts[2].count > 8 {
            let tail = Array(parts.dropFirst(3))
            return proposeReply(id: parts[2], tail: tail, method: method)
                ?? cabalReply(id: parts[2], tail: tail, range: range, script: script)
        }
        if parts.count >= 2, parts[1] == "assets" {
            return assetReply(tail: Array(parts.dropFirst(2)), query: query, script: script)
        }
        if path.hasPrefix("/v1/me") { return meReply(path: path, script: script) }
        switch path {
        case "/v1/leaderboards/people", "/v1/leaderboards/cabals":
            return json(Components.Schemas.LeaderboardPage.samplePeople(count: empty ? 0 : 3))
        case "/v1/feed":
            let items = empty ? [] : Components.Schemas.FeedItem.samples
            return json(Components.Schemas.FeedPage(items: items, nextCursor: nil))
        default:
            return problem(404, "not_found", "Not in the sample data.")
        }
    }

    private static func meReply(path: String, script: SampleAPIScript) -> Reply {
        let empty = script.mode == .empty
        switch path {
        case "/v1/me":
            return json(Components.Schemas.Me.sample)
        case "/v1/me/cabals":
            return empty ? raw("[]") : json(myCabals(script))
        case "/v1/me/balance":
            return script.mode == .balanceFails
                ? problem(503, "unavailable", "The balance can't be read right now.") : json(script.balance)
        case "/v1/me/portfolio":
            return json(empty ? Components.Schemas.MyPortfolio.sampleEmpty : .sample)
        case "/v1/me/pnl-history":
            return json(empty ? Components.Schemas.MyPnlHistory.sampleEmpty : .sample())
        case "/v1/me/txns":
            return json(empty ? Components.Schemas.UserTxnPage.sampleEmpty : .sampleFirst)
        case "/v1/me/pending-votes", "/v1/me/cabal-invites":
            return raw("[]")
        default:
            return problem(404, "not_found", "Not in the sample data.")
        }
    }

    private static func myCabals(_ script: SampleAPIScript) -> [Components.Schemas.MyCabal] {
        (0..<script.cabalCount).map { index in
            index == 0
                ? myCabal(id: Components.Schemas.Cabal.sample(role: nil).id, script)
                : myCabal(id: CabalsTabSampleData.createdCabalID, name: "Friday Fund", script)
        }
    }

    private static func assetReply(tail: [String], query: [String: String], script: SampleAPIScript) -> Reply {
        let chartOnly = script.mode == .chartUnavailable && tail.last == "chart"
        if script.mode == .assetsUnavailable || chartOnly {
            return problem(503, "unavailable", "Stocks can't be read right now.")
        }
        switch tail.count {
        case 0:
            return json(assetList(query: query, empty: script.mode == .empty))
        case 1:
            return json(script.asset)
        case 2 where tail[1] == "chart":
            return json(script.chart)
        default:
            return problem(404, "not_found", "Not in the sample data.")
        }
    }

    private static func assetList(query: [String: String], empty: Bool) -> Components.Schemas.AssetList {
        if empty { return .init(assets: [], nextCursor: nil) }
        let catalog: [Components.Schemas.AssetSummary] = [.googl, .spaceX, .unpriced]
        if let text = query["q"], !text.isEmpty {
            let matches = catalog.filter {
                $0.symbol.localizedCaseInsensitiveContains(text)
                    || $0.displayName.localizedCaseInsensitiveContains(text)
            }
            return .init(assets: matches, nextCursor: nil)
        }
        switch query["filter"] {
        case "popular": return .popularSample
        case "pre_ipo": return .preIpoSample
        default: return .init(assets: catalog, nextCursor: nil)
        }
    }

    private static func myCabal(
        id: String, name: String = "QA pot", _ script: SampleAPIScript
    ) -> Components.Schemas.MyCabal {
        Components.Schemas.MyCabal(
            id: id, name: name, pictureUrl: script.pictureURL, role: script.role, canVote: true, memberCount: 3,
            joinedAt: Date(timeIntervalSince1970: 1_790_000_000), pendingRequestCount: 0, unreadCount: 0)
    }

    private static func proposeReply(id: String, tail: [String], method: String) -> Reply? {
        if tail == ["proposals", "preview"] { return json(Components.Schemas.TradePreview.proposalPreviewClean) }
        guard tail == ["proposals"], method == "POST" else { return nil }
        var created = Components.Schemas.Proposal.sample()
        created.cabalId = id
        var reply = json(created)
        reply.status = 201
        return reply
    }

    private static func cabalReply(id: String, tail: [String], range: String?, script: SampleAPIScript) -> Reply {
        let empty = script.mode == .empty
        switch tail.first {
        case nil:
            var cabal = Components.Schemas.Cabal.sampleWithMembers(role: script.role)
            cabal.id = id
            cabal.pictureUrl = script.pictureURL
            if id == CabalsTabSampleData.createdCabalID, let name = createdName.withLock({ $0 }) { cabal.name = name }
            return json(cabal)
        case "pot":
            if script.mode == .potUnavailable { return problem(503, "unavailable", "The pot can't be read right now.") }
            var pot = empty ? Components.Schemas.CabalPot.sampleZero : script.pot
            pot.cabalId = id
            return json(pot)
        case "proposals":
            return json(
                Components.Schemas.ProposalList(
                    proposals: empty ? [] : [Components.Schemas.Proposal.sample()], nextCursor: nil))
        case "activity":
            return json(empty ? Components.Schemas.CabalActivityPage.sampleEmpty : .sampleFirst)
        case "leaderboard":
            return json(Components.Schemas.LeaderboardPage.samplePeople(count: empty ? 0 : 3))
        case "value-history":
            let history: Components.Schemas.CabalValueHistory =
                range == "1D"
                ? .init(cabalId: id, range: ._1d, points: [], pricesAsOf: nil) : .sample(cabalID: id)
            return json(history)
        case "access-requests", "invites":
            return raw("[]")
        default:
            return problem(404, "not_found", "Not in the sample data.")
        }
    }
}
#endif
