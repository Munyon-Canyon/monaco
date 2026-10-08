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
    }

    var mode = Mode.populated
    var role = "creator"
    var pictureURL: String?
    var pictureWriteFails = false
    var balance = Components.Schemas.Balance.sample
}

nonisolated final class SampleAPIProtocol: URLProtocol {
    private static let script = Mutex(SampleAPIScript())
    private static let registered = Mutex(false)

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
        let reply = Self.reply(path: url.path, method: request.httpMethod ?? "GET", script: script)
        let response = HTTPURLResponse(
            url: url, statusCode: reply.status, httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": reply.contentType]
        )
        guard let response else { return }
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: reply.body)
        client?.urlProtocolDidFinishLoading(self)
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

    private static func reply(path: String, method: String, script: SampleAPIScript) -> Reply {
        let empty = script.mode == .empty
        let parts = path.split(separator: "/").map(String.init)
        if method == "PUT" || method == "DELETE", parts.last == "picture", script.pictureWriteFails {
            return problem(413, "picture_invalid", "Picture must be at most 2MB.")
        }
        if parts.count >= 3, parts[1] == "cabals", UUID(uuidString: parts[2]) != nil || parts[2].count > 8 {
            return cabalReply(id: parts[2], tail: Array(parts.dropFirst(3)), script: script)
        }
        if path.hasPrefix("/v1/me") { return meReply(path: path, script: script) }
        switch path {
        case "/v1/leaderboards/people", "/v1/leaderboards/cabals":
            return json(Components.Schemas.LeaderboardPage.samplePeople(count: empty ? 0 : 3))
        case "/v1/assets":
            return json(Components.Schemas.AssetList.allSample)
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
            return empty ? raw("[]") : json([myCabal(id: Components.Schemas.Cabal.sample(role: nil).id, script)])
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

    private static func myCabal(id: String, _ script: SampleAPIScript) -> Components.Schemas.MyCabal {
        Components.Schemas.MyCabal(
            id: id, name: "QA pot", pictureUrl: script.pictureURL, role: script.role, canVote: true, memberCount: 3,
            joinedAt: Date(timeIntervalSince1970: 1_790_000_000), pendingRequestCount: 0, unreadCount: 0)
    }

    private static func cabalReply(id: String, tail: [String], script: SampleAPIScript) -> Reply {
        let empty = script.mode == .empty
        switch tail.first {
        case nil:
            var cabal = Components.Schemas.Cabal.sampleWithMembers(role: script.role)
            cabal.id = id
            cabal.pictureUrl = script.pictureURL
            return json(cabal)
        case "pot":
            var pot = empty ? Components.Schemas.CabalPot.sampleZero : .sampleInvested
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
            return json(Components.Schemas.CabalValueHistory.sample(cabalID: id))
        case "access-requests", "invites":
            return raw("[]")
        default:
            return problem(404, "not_found", "Not in the sample data.")
        }
    }
}
#endif
