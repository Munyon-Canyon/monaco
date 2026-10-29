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
        case offline
    }

    enum CabalBoardMode: Sendable {
        case rows
        case empty
        case failing
        case hanging
    }

    var mode = Mode.populated
    var cabalBoardMode = CabalBoardMode.rows
    var role: String? = "creator"
    var pictureURL: String?
    var pictureWriteFails = false
    var balance = Components.Schemas.Balance.sample
    var asset = Components.Schemas.AssetDetail.googl
    var chart = Components.Schemas.AssetChart.candles
    var cabalCount = 1
    var candleCount = 300
    var sparseCandles = false
    var pot = Components.Schemas.CabalPot.sampleInvested
    var proposal: Components.Schemas.ProposalDetail?
    var vote = VoteAnswer.ordinary
    var undo = UndoAnswer.accepted
    var pendingVotes: [Components.Schemas.PendingVote] = []
    var cashOut = Components.Schemas.CashOutPreview.sample
    var invites: [Components.Schemas.CabalInvite] = []
    var accessRequests: [Components.Schemas.CabalAccessRequest] = []
    var viewerCanVote = false
    var proposalStatus = Components.Schemas.ProposalStatus.open
    var hidesProposals = false
    var accessRequest: (direction: String, status: String)?
    var joinMode: String?
    var unreadCount = 0
    var includesPausedStock = false
    var portfolio: Components.Schemas.MyPortfolio?
    var pnlHistory: Components.Schemas.MyPnlHistory?
    var homeCabals: [Components.Schemas.MyCabal] = []
    var ownershipBps: [String: Int32] = [:]
    var proposals: [String: Components.Schemas.ProposalDetail] = [:]
    var assets: [String: Components.Schemas.AssetDetail] = [:]
    var cabalBoard: [Components.Schemas.LeaderboardRow] = []
    var portfolioReadsBeforeFailing: Int?
    var pendingVotesFail = false
    var pendingVotesHang = false
    var closedProposals: [Components.Schemas.Proposal] = []
    var closedProposalsFail = false
    var closedProposalsHang = false
    var catalog: [Components.Schemas.AssetSummary] = []
    var failsAfterAssetReads: Int?
    var invitePreview = InvitePreview.frame
    var held: Components.Schemas.HeldList?
    var assetFeed: [Components.Schemas.FeedItem] = []
    var cabalEdit: (@Sendable (inout Components.Schemas.Cabal) -> Void)?
    var openProposal: Components.Schemas.Proposal?
    var valueHistory: Components.Schemas.CabalValueHistory?

    enum VoteAnswer: Sendable, Equatable {
        case ordinary
        case settles
        case refused(status: Int, code: String, message: String)
        case dropped
    }

    enum UndoAnswer: Sendable, Equatable {
        case accepted
        case refused(status: Int, code: String, message: String)
        case dropped
    }

    enum InvitePreview: Sendable {
        case frame
        case noProposal
        case hang
        case fail
    }
}

nonisolated final class SampleAPIProtocol: URLProtocol {
    private static let script = Mutex(SampleAPIScript())
    private static let registered = Mutex(false)
    private static let createdName = Mutex<String?>(nil)
    private static let castChoice = Mutex<String?>(nil)
    private static let postedComments = Mutex<[Components.Schemas.Comment]>([])
    private static let portfolioReads = Mutex(0)
    private static let assetReads = Mutex(0)

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
        if Self.hangs(url.path, script) { return }
        if script.mode == .offline {
            client?.urlProtocol(self, didFailWithError: URLError(.notConnectedToInternet))
            return
        }
        let method = request.httpMethod ?? "GET"
        if method == "POST", url.path == "/v1/cabals" {
            Self.createdName.withLock { $0 = requestObject()?["name"] as? String }
        }
        if method == "POST", url.path.hasSuffix("/votes") {
            if script.vote == .dropped {
                client?.urlProtocol(self, didFailWithError: URLError(.networkConnectionLost))
                return
            }
            Self.castChoice.withLock { $0 = requestObject()?["choice"] as? String }
        }
        if method == "DELETE", url.path.hasSuffix("/votes"), script.undo == .dropped {
            client?.urlProtocol(self, didFailWithError: URLError(.networkConnectionLost))
            return
        }
        if method == "POST", url.path.hasSuffix("/comments") {
            Self.recordComment(requestObject())
        }
        let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems ?? []
        let query = Dictionary(items.compactMap { item in item.value.map { (item.name, $0) } }) { first, _ in first }
        let reply =
            Self.cabalBoardModeReply(url.path, script)
            ?? Self.invitePreviewReply(path: url.path, script: script)
            ?? Self.reply(path: url.path, method: method, query: query, script: script)
        let response = HTTPURLResponse(
            url: url, statusCode: reply.status, httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": reply.contentType].merging(reply.headers) { first, _ in first }
        )
        guard let response else { return }
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: reply.body)
        client?.urlProtocolDidFinishLoading(self)
    }

    private static func hangs(_ path: String, _ script: SampleAPIScript) -> Bool {
        (script.invitePreview == .hang && path.hasPrefix("/v1/referrals/"))
            || (script.pendingVotesHang && path == "/v1/me/pending-votes")
            || (script.closedProposalsHang && path == "/v1/me/proposals")
            || (script.cabalBoardMode == .hanging && path == "/v1/leaderboards/cabals")
    }

    private static func cabalBoardModeReply(_ path: String, _ script: SampleAPIScript) -> Reply? {
        guard path == "/v1/leaderboards/cabals" else { return nil }
        switch script.cabalBoardMode {
        case .failing:
            return problem(503, "unavailable", "Rankings can't be read right now.")
        case .empty:
            var page = Components.Schemas.LeaderboardPage.samplePeople(count: 0)
            page.board = "cabals"
            return json(page)
        case .rows, .hanging:
            return nil
        }
    }

    private func requestObject() -> [String: Any]? {
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
        guard let body else { return nil }
        return try? JSONSerialization.jsonObject(with: body) as? [String: Any]
    }

    private struct Reply {
        var status = 200
        var contentType = "application/json"
        var headers: [String: String] = [:]
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
            return proposeReply(id: parts[2], tail: tail, method: method, query: query, script: script)
                ?? membersReply(id: parts[2], tail: tail, method: method, script: script)
                ?? cabalReply(id: parts[2], tail: tail, range: range, script: script)
        }
        if parts.count >= 3, parts[1] == "proposals" {
            return proposalReply(id: parts[2], tail: Array(parts.dropFirst(3)), method: method, script: script)
        }
        if parts.count >= 2, parts[1] == "assets" {
            return assetReply(tail: Array(parts.dropFirst(2)), query: query, script: script)
        }
        if path.hasPrefix("/v1/me") { return meReply(path: path, script: script) }
        switch path {
        case "/v1/leaderboards/cabals" where !script.cabalBoard.isEmpty:
            var page = Components.Schemas.LeaderboardPage.samplePeople(count: 0)
            page.board = "cabals"
            page.rows = script.cabalBoard
            return json(page)
        case "/v1/leaderboards/people", "/v1/leaderboards/cabals":
            return json(Components.Schemas.LeaderboardPage.samplePeople(count: empty ? 0 : 3))
        case "/v1/feed":
            return feedReply(query: query, script: script, empty: empty)
        default:
            if path.hasPrefix("/v1/feed/") { return feedItemReply(id: String(path.dropFirst("/v1/feed/".count))) }
            return problem(404, "not_found", "Not in the sample data.")
        }
    }
}

nonisolated extension SampleAPIProtocol {
    private static func invitePreviewReply(path: String, script: SampleAPIScript) -> Reply? {
        let parts = path.split(separator: "/").map(String.init)
        guard parts.count == 5, parts[1] == "referrals", parts[3] == "cabals" else { return nil }
        let cabalID = parts[4]
        guard script.invitePreview != .fail else {
            return problem(503, "unavailable", "The invite can't be read right now.")
        }
        var preview =
            script.invitePreview == .noProposal
            ? Components.Schemas.ReferralCabal.sampleNoOpenProposal : .sample
        preview.cabal?.id = cabalID
        preview.cabal?.name = Components.Schemas.Cabal.sample(role: nil).name
        var reply = json(preview)
        reply.headers["Cache-Control"] = "no-cache"
        return reply
    }

    private static func feedItemReply(id: String) -> Reply {
        var item = Components.Schemas.FeedItem.samples[0]
        item.id = id
        return json(Components.Schemas.FeedItemDetail(item: item, visible: true, canComment: true))
    }

    private static func meReply(path: String, script: SampleAPIScript) -> Reply {
        let empty = script.mode == .empty
        switch path {
        case "/v1/me":
            return json(Components.Schemas.Me.sample)
        case "/v1/me/cabals":
            return myCabalsReply(script)
        case "/v1/me/balance":
            return script.mode == .balanceFails
                ? problem(503, "unavailable", "The balance can't be read right now.") : json(script.balance)
        case "/v1/me/referral-code":
            return json(Components.Schemas.MyReferralCode.sample)
        case "/v1/me/portfolio":
            return portfolioReply(script)
        case "/v1/me/pnl-history":
            return json(script.pnlHistory ?? (empty ? Components.Schemas.MyPnlHistory.sampleEmpty : .sampleCurve()))
        case "/v1/me/txns":
            return json(empty ? Components.Schemas.UserTxnPage.sampleEmpty : .sampleFirst)
        case "/v1/me/market-facts":
            return json(script.catalog.isEmpty ? Components.Schemas.MarketFacts.sampleEmpty : .sampleAndres)
        case "/v1/me/pending-votes":
            return pendingVotesReply(script)
        case "/v1/me/proposals":
            return closedProposalsReply(script)
        case "/v1/me/cabal-invites":
            return json(script.invites)
        default:
            return path == "/v1/me/held" ? heldReply(script) : problem(404, "not_found", "Not in the sample data.")
        }
    }

    private static func heldReply(_ script: SampleAPIScript) -> Reply {
        guard let held = script.held else { return problem(404, "not_found", "Not in the sample data.") }
        return json(held)
    }

    private static func feedReply(query: [String: String], script: SampleAPIScript, empty: Bool) -> Reply {
        let items: [Components.Schemas.FeedItem]
        if let symbol = query["symbol"], !script.assetFeed.isEmpty {
            let status = query["status"]
            items = script.assetFeed.filter { $0.symbol == symbol && (status == nil || $0.status == status) }
        } else {
            items = empty ? [] : Components.Schemas.FeedItem.samples
        }
        return json(Components.Schemas.FeedPage(items: items, nextCursor: nil))
    }

    private static func myCabalsReply(_ script: SampleAPIScript) -> Reply {
        if !script.homeCabals.isEmpty { return json(script.homeCabals) }
        return script.mode == .empty ? raw("[]") : json(myCabals(script))
    }

    private static func portfolioReply(_ script: SampleAPIScript) -> Reply {
        let reads = portfolioReads.withLock { count in
            count += 1
            return count
        }
        if let allowed = script.portfolioReadsBeforeFailing, reads > allowed {
            return problem(503, "unavailable", "The portfolio can't be read right now.")
        }
        return json(script.portfolio ?? (script.mode == .empty ? .sampleEmpty : .sample))
    }

    private static func pendingVotesReply(_ script: SampleAPIScript) -> Reply {
        if script.pendingVotesFail { return problem(503, "unavailable", "Proposals can't be read right now.") }
        return json(script.pendingVotes)
    }

    private static func closedProposalsReply(_ script: SampleAPIScript) -> Reply {
        if script.closedProposalsFail { return problem(503, "unavailable", "Proposals can't be read right now.") }
        return json(Components.Schemas.ProposalList(proposals: script.closedProposals, nextCursor: nil))
    }

    private static func potReply(id: String, script: SampleAPIScript) -> Reply {
        if script.mode == .potUnavailable { return problem(503, "unavailable", "The pot can't be read right now.") }
        var pot = script.mode == .empty ? Components.Schemas.CabalPot.sampleZero : script.pot
        pot.cabalId = id
        if let bps = script.ownershipBps[id] { pot.me?.sliceBps = bps }
        return json(pot)
    }

    private static func myCabals(_ script: SampleAPIScript) -> [Components.Schemas.MyCabal] {
        (0..<script.cabalCount).map { index in
            index == 0
                ? myCabal(id: Components.Schemas.Cabal.sample(role: nil).id, script)
                : myCabal(id: CabalsTabSampleData.createdCabalID, name: "Friday Fund", script)
        }
    }

    private static func proposalReply(id: String, tail: [String], method: String, script: SampleAPIScript) -> Reply {
        guard var proposal = script.proposals[id] ?? script.proposal else {
            return problem(503, "unavailable", "Proposals can't be read right now.")
        }
        proposal.id = id
        switch (method, tail.first) {
        case ("GET", nil):
            return json(castApplied(proposal, script))
        case ("GET", "votes"):
            return votesReply(proposal)
        case ("POST", "votes"):
            if case .refused(let status, let code, let message) = script.vote {
                castChoice.withLock { $0 = nil }
                return problem(status, code, message)
            }
            let choice = Components.Schemas.BallotChoice(rawValue: castChoice.withLock { $0 } ?? "") ?? .yes
            let cast = castApplied(proposal, script, choice: choice)
            let settles = script.vote == .settles
            return json(
                Components.Schemas.VoteResult(
                    myVote: cast.myVote.map {
                        .init(
                            order: $0.order, of: $0.of, yesOrder: $0.yesOrder, noOrder: $0.noOrder,
                            decided: $0.decided)
                    } ?? .init(order: 1, of: 1, yesOrder: 0, noOrder: 0, decided: false),
                    proposalId: proposal.id, status: cast.status, tally: cast.tally, myBallot: choice,
                    undoUntil: settles ? nil : Date(timeIntervalSinceNow: 15)))
        case ("DELETE", "votes"):
            if case .refused(let status, let code, let message) = script.undo {
                return problem(status, code, message)
            }
            castChoice.withLock { $0 = nil }
            return json(
                Components.Schemas.RetractVoteResult(
                    proposalId: proposal.id, status: proposal.status, tally: proposal.tally))
        case ("GET", "comments"):
            return json(
                Components.Schemas.ProposalCommentPage(
                    feedObjectId: "01920000-0000-7000-8000-000000000007",
                    items: threadsWithPosted(Components.Schemas.CommentThread.samples), nextCursor: nil))
        case ("POST", "comments"):
            guard let posted = postedComments.withLock({ $0.last }) else {
                return problem(400, "invalid_request", "No comment in the request.")
            }
            var reply = json(posted)
            reply.status = 201
            return reply
        default:
            return problem(404, "not_found", "Not in the sample data.")
        }
    }

    private static func castApplied(
        _ detail: Components.Schemas.ProposalDetail, _ script: SampleAPIScript,
        choice: Components.Schemas.BallotChoice? = nil
    ) -> Components.Schemas.ProposalDetail {
        let sent = castChoice.withLock { $0 }.flatMap(Components.Schemas.BallotChoice.init(rawValue:))
        guard let choice = choice ?? sent else { return detail }
        var cast = detail
        if choice == .yes { cast.tally.yes += 1 } else { cast.tally.no += 1 }
        cast.myBallot = .init(rawValue: choice.rawValue)
        let settles = script.vote == .settles
        cast.myVote = .init(
            order: cast.tally.yes + cast.tally.no, of: cast.tally.voters, yesOrder: choice == .yes ? cast.tally.yes : 0,
            noOrder: choice == .no ? cast.tally.no : 0, decided: settles)
        if settles {
            cast.status = .passed
            cast.canVote = false
            cast.stats.closedAt = .now
        }
        return cast
    }

    private static func recordComment(_ object: [String: Any]?) {
        guard let body = object?["body"] as? String else { return }
        let parent = object?["parent_comment_id"] as? String
        let id = "posted-\(postedComments.withLock { $0.count + 1 })"
        let comment = Components.Schemas.Comment(
            id: id, parentCommentId: parent,
            author: .init(
                id: "01890a5d-ac96-774b-bcce-b302099a8058", handle: "kaicenat", displayName: "Kai Cenat",
                photoUrl: nil, returnPrivate: false),
            replyToHandle: nil, body: body, bodyDisplay: body, isMine: true, reactions: [], reactionCount: 0,
            createdAt: .now)
        postedComments.withLock { $0.append(comment) }
    }

    private static func threadsWithPosted(
        _ samples: [Components.Schemas.CommentThread]
    ) -> [Components.Schemas.CommentThread] {
        let posted = postedComments.withLock { $0 }
        var threads = samples
        for comment in posted {
            if let parent = comment.parentCommentId,
                let index = threads.firstIndex(where: { $0.comment.id == parent })
            {
                threads[index].replies.append(comment)
            } else {
                threads.append(Components.Schemas.CommentThread(comment: comment, replies: []))
            }
        }
        return threads
    }

    private static func assetReply(tail: [String], query: [String: String], script: SampleAPIScript) -> Reply {
        let chartOnly = script.mode == .chartUnavailable && tail.last == "chart"
        let exhausted = script.failsAfterAssetReads.map { limit in
            tail.count <= 1
                && assetReads.withLock { reads in
                    reads += 1
                    return reads > limit
                }
        }
        if script.mode == .assetsUnavailable || chartOnly || exhausted == true {
            return problem(503, "unavailable", "Stocks can't be read right now.")
        }
        switch tail.count {
        case 0:
            return json(
                assetList(
                    query: query, empty: script.mode == .empty, paused: script.includesPausedStock, script: script))
        case 1:
            if let detail = script.assets[tail[0]] { return json(detail) }
            let listed = script.catalog.first { $0.symbol == tail[0] }
            return json(listed.map(Components.Schemas.AssetDetail.init(listing:)) ?? script.asset)
        case 2 where tail[1] == "chart":
            return json(
                chartReply(script.chart, query: query, candleCount: script.candleCount, sparse: script.sparseCandles))
        default:
            return problem(404, "not_found", "Not in the sample data.")
        }
    }

    private static func chartReply(
        _ base: Components.Schemas.AssetChart, query: [String: String], candleCount: Int, sparse: Bool
    ) -> Components.Schemas.AssetChart {
        let range = query["range"].flatMap(Components.Schemas.AssetChart.RangePayload.init(rawValue:)) ?? base.range
        let interval =
            query["interval"].flatMap(Components.Schemas.AssetChart.IntervalPayload.init(rawValue:))
            ?? Self.defaultInterval(range)
        if query["before"] != nil {
            return .init(
                range: range, interval: interval, bucketSeconds: base.bucketSeconds, points: [], empty: true,
                attribution: base.attribution, nextBefore: nil)
        }
        if base == .candles { return .candles(range: range, interval: interval, count: candleCount, sparse: sparse) }
        var echoed = base
        echoed.range = range
        echoed.interval = interval
        return echoed
    }

    private static func defaultInterval(
        _ range: Components.Schemas.AssetChart.RangePayload
    ) -> Components.Schemas.AssetChart.IntervalPayload {
        switch range {
        case ._1h: ._1m
        case ._1d: ._5m
        case ._1w, ._1m, ._3m: ._1h
        case ._6m, ._1y, .all: ._1d
        }
    }

    private static func assetList(
        query: [String: String], empty: Bool, paused: Bool = false, script: SampleAPIScript
    ) -> Components.Schemas.AssetList {
        if empty { return .init(assets: [], nextCursor: nil) }
        let catalog: [Components.Schemas.AssetSummary] =
            script.catalog.isEmpty
            ? (paused ? [.paused, .googl, .spaceX, .unpriced] : [.googl, .spaceX, .unpriced]) : script.catalog
        if let text = query["q"], !text.isEmpty {
            let matches = catalog.filter {
                $0.symbol.localizedCaseInsensitiveContains(text)
                    || $0.displayName.localizedCaseInsensitiveContains(text)
            }
            return .init(assets: matches, nextCursor: nil)
        }
        switch query["filter"] {
        case "popular" where !script.catalog.isEmpty: return .init(assets: catalog, nextCursor: nil)
        case "popular": return paused ? .init(assets: [.paused, .googl], nextCursor: nil) : .popularSample
        case "pre_ipo" where !script.catalog.isEmpty:
            return .init(assets: catalog.filter { $0.kind == .preIpo }, nextCursor: nil)
        case "pre_ipo": return .preIpoSample
        default: return .init(assets: catalog, nextCursor: nil)
        }
    }

    private static func myCabal(
        id: String, name: String = "QA pot", _ script: SampleAPIScript
    ) -> Components.Schemas.MyCabal {
        Components.Schemas.MyCabal(
            id: id, name: name, pictureUrl: script.pictureURL, role: script.role ?? "member", canVote: true,
            memberCount: 3,
            joinedAt: Date(timeIntervalSince1970: 1_790_000_000), pendingRequestCount: 0,
            unreadCount: script.unreadCount)
    }

    private static func proposeReply(
        id: String, tail: [String], method: String, query: [String: String], script: SampleAPIScript
    ) -> Reply? {
        if tail == ["proposals", "preview"] {
            if script.mode == .potUnavailable, query["usdc_micros"] == nil {
                return problem(503, "unavailable", "The cash can't be read right now.")
            }
            return json(Components.Schemas.TradePreview.proposalPreviewClean)
        }
        guard tail == ["proposals"], method == "POST" else { return nil }
        var created = Components.Schemas.ProposalDetail.sample()
        created.cabalId = id
        var reply = json(created)
        reply.status = 201
        return reply
    }

    private static func cabalRead(id: String, script: SampleAPIScript) -> Components.Schemas.Cabal {
        var cabal = Components.Schemas.Cabal.sampleWithMembers(role: script.role)
        cabal.id = id
        cabal.pictureUrl = script.pictureURL
        if let invite = script.invites.first(where: { $0.cabal.id == id }) {
            cabal.myAccessRequest = .init(id: invite.requestId, direction: "invite", status: "pending")
        }
        if let request = script.accessRequest {
            cabal.myAccessRequest = .init(id: "sample-request", direction: request.direction, status: request.status)
        }
        if let joinMode = script.joinMode { cabal.rules.joinMode = joinMode }
        if id == CabalsTabSampleData.createdCabalID, let name = createdName.withLock({ $0 }) { cabal.name = name }
        script.cabalEdit?(&cabal)
        return cabal
    }

    private static func membersReply(id: String, tail: [String], method: String, script: SampleAPIScript) -> Reply? {
        guard method == "GET", tail == ["members"] else { return nil }
        let members = cabalRead(id: id, script: script).members
        return json(Components.Schemas.CabalMemberPage.sample(members: members))
    }

    private static func votesReply(_ proposal: Components.Schemas.ProposalDetail) -> Reply {
        let members = Components.Schemas.Cabal.sampleWithMembers(role: "member").members
        let items = proposal.voters.map { voter in
            let member = members.first { $0.userId == voter.userId }
            return Components.Schemas.ProposalVoteItem(
                userId: voter.userId, voted: voter.choice != nil, handle: member?.handle,
                displayName: member?.displayName ?? "Member", photoUrl: member?.photoUrl,
                choice: voter.choice.flatMap { .init(rawValue: $0.rawValue) }, castAt: voter.castAt)
        }
        return json(Components.Schemas.ProposalVotePage(items: items, nextCursor: nil))
    }

    private static func cabalReply(id: String, tail: [String], range: String?, script: SampleAPIScript) -> Reply {
        let empty = script.mode == .empty
        switch tail.first {
        case nil:
            return json(cabalRead(id: id, script: script))
        case "pot":
            return potReply(id: id, script: script)
        case "proposals":
            var proposal = script.openProposal ?? .sample(status: script.proposalStatus)
            proposal.canVote = script.viewerCanVote
            return json(
                Components.Schemas.ProposalList(
                    proposals: empty || script.hidesProposals ? [] : [proposal], nextCursor: nil))
        case "activity":
            return json(empty ? Components.Schemas.CabalActivityPage.sampleEmpty : .sampleFirst)
        case "leaderboard":
            return json(Components.Schemas.LeaderboardPage.samplePeople(count: empty ? 0 : 3))
        case "value-history":
            let history: Components.Schemas.CabalValueHistory =
                empty || range == "1D"
                ? .init(cabalId: id, range: ._1d, points: [], pricesAsOf: nil)
                : script.valueHistory ?? .sampleCurve(cabalID: id)
            return json(history)
        case "cashouts" where tail.last == "preview":
            return json(script.cashOut)
        case "access-requests":
            return json(script.accessRequests)
        case "messages":
            return json(Components.Schemas.ChatChannelPage(messages: empty ? [] : [Self.sampleChatMessage]))
        case "invites":
            return raw("[]")
        case "picture":
            var cabal = Components.Schemas.Cabal.sampleWithMembers(role: script.role)
            cabal.id = id
            cabal.pictureUrl = nil
            return json(cabal)
        default:
            return problem(404, "not_found", "Not in the sample data.")
        }
    }

    private static var sampleChatMessage: Components.Schemas.ChatMessage {
        Components.Schemas.ChatMessage(
            id: "sample-message",
            author: .init(id: "sample-author", handle: "alex", displayName: "Alex", photoUrl: nil),
            body: "What\u{2019}s everyone watching this week?",
            createdAt: Date(timeIntervalSinceNow: -720), alsoInChannel: false, replyCount: 0, deleted: false)
    }
}
#endif
