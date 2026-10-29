import Foundation
import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import OpenAPIRuntime
import XCTest

@testable import MonacoCore

@MainActor
final class ShareScreenModelTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_000_000)
    private let subject = ShareSubject.position(cabalID: "cabal-1", symbol: "NVDAx")

    func testThePositionLoadsFromAllFiveReads() async throws {
        let server = ShareServer(tradePages: [3])
        let model = make(server)

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.header.title, Components.Schemas.Cabal.sample(role: "member").name)
        XCTAssertEqual(spec.header.subtitle, "Holding · 3 calls")
        XCTAssertEqual(spec.tickerLine.symbol, "NVDA")
        XCTAssertEqual(spec.figure.main, "+$1,027.90")
        let paths = await server.paths
        XCTAssertEqual(
            Set(paths),
            [
                "/v1/cabals/cabal-1/pot", "/v1/cabals/cabal-1", "/v1/cabals/cabal-1/activity",
                "/v1/assets/NVDAx/chart", "/v1/me/referral-code",
            ])
    }

    func testTheChartRangeCoversTheFirstBuy() async throws {
        let server = ShareServer(tradePages: [1])
        let model = make(server)

        await model.load()

        let queries = await server.chartQueries
        XCTAssertEqual(queries.count, 1)
        XCTAssertTrue(queries[0].contains("range=3M"), queries[0])
    }

    func testA250TradePositionLoadsEveryPageAndCountsAll() async throws {
        let server = ShareServer(tradePages: [100, 100, 50])
        let model = make(server)

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.header.subtitle, "Holding · 250 calls")
        XCTAssertEqual(spec.chart.markers.count, 250)
        let pages = await server.activityCursors
        XCTAssertEqual(pages, [nil, "page-1", "page-2"])
    }

    func testThePageStopTruncatesAndSaysSo() async throws {
        let server = ShareServer(tradePages: Array(repeating: 1, count: 5), endless: true)
        let model = make(server, maxPages: 3)

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.header.subtitle, "Holding · 3+ calls")
        let pages = await server.activityCursors
        XCTAssertEqual(pages.count, 3)
    }

    func testAnEyeFlipRebuildsTheSpecWithoutAnotherRead() async throws {
        let server = ShareServer(tradePages: [3])
        let model = make(server)
        await model.load()
        let before = await server.paths.count

        model.eyes.pnlDollars = false
        XCTAssertEqual(model.spec?.figure.main, "+14.0%")
        model.eyes.buySell = false
        XCTAssertEqual(model.spec?.chart.markers.count, 0)
        model.eyes.code = false
        XCTAssertNil(model.spec?.code)

        let after = await server.paths.count
        XCTAssertEqual(after, before)
    }

    func testAFailedReadIsTheFailedState() async {
        let server = ShareServer(tradePages: [3], failing: "/v1/cabals/cabal-1/pot")
        let model = make(server)

        await model.load()

        guard case .failed = model.state else { return XCTFail("got \(model.state)") }
        XCTAssertNil(model.spec)
        XCTAssertNil(model.shareURL)
    }

    func testAMissingPositionIsTheFailedState() async {
        let model = make(ShareServer(tradePages: [3]), subject: .position(cabalID: "cabal-1", symbol: "TSLAx"))

        await model.load()

        guard case .failed = model.state else { return XCTFail("got \(model.state)") }
    }

    func testTheShareURLCarriesTheCabalAndTheHandle() async throws {
        let model = make(ShareServer(tradePages: [1]))

        await model.load()

        let url = try XCTUnwrap(model.shareURL)
        XCTAssertEqual(url.path, "/r/kaicenat")
        XCTAssertEqual(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first?.name, "c")
        XCTAssertEqual(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first?.value, "cabal-1")
        XCTAssertEqual(model.spec?.code, "KAICENAT")
    }

    func testALockedHandleFallsBackToTheRandomCode() async throws {
        let model = make(ShareServer(tradePages: [1], locked: true))

        await model.load()

        XCTAssertEqual(model.spec?.code, "K7M4QX2P")
        XCTAssertEqual(model.shareURL?.path, "/r/k7m4qx2p")
    }

    func testAnOpenVoteLoadsTheVoteCardWithTheProposalLink() async throws {
        let server = ShareServer(tradePages: [1], proposal: .openNVDA(now: now))
        let model = make(server, subject: .openVote(id: "proposal-1"))

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.replayTitle, "Replay the price")
        XCTAssertEqual(model.title, "Share vote")
        XCTAssertEqual(model.format, .square)
        XCTAssertEqual(model.eyes, ShareEyes(pnlDollars: false, buySell: false))
        let items = URLComponents(url: try XCTUnwrap(model.shareURL), resolvingAgainstBaseURL: false)?.queryItems
        XCTAssertEqual(items?.first { $0.name == "c" }?.value, Components.Schemas.Cabal.sample(role: nil).id)
        XCTAssertEqual(items?.first { $0.name == "p" }?.value, "proposal-1")
        let paths = await server.paths
        XCTAssertEqual(
            Set(paths),
            [
                "/v1/proposals/proposal-1", "/v1/cabals/\(Components.Schemas.Cabal.sample(role: nil).id)",
                "/v1/assets/NVDAx/chart", "/v1/me/referral-code",
            ])
        let queries = await server.chartQueries
        XCTAssertTrue(queries[0].contains("range=1D"), queries[0])
    }

    func testAPassedProposalLoadsShareB() async throws {
        let server = ShareServer(tradePages: [1], proposal: .filledMETA(now: now))
        let model = make(server, subject: .passedProposal(id: "proposal-1"))

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.replayTitle, "Replay the call")
        XCTAssertEqual(model.title, "Share")
        XCTAssertEqual(model.eyes, ShareEyes(pnlDollars: false))
        XCTAssertEqual(model.shareURL?.query, "c=\(Components.Schemas.Cabal.sample(role: nil).id)")
    }

    func testAStaleEntryPointFallsBackToTheOtherBuilder() async throws {
        let closed = make(
            ShareServer(tradePages: [1], proposal: .filledMETA(now: now)), subject: .openVote(id: "proposal-1"))
        await closed.load()
        XCTAssertEqual(closed.spec?.replayTitle, "Replay the call")
        XCTAssertEqual(closed.title, "Share")

        let open = make(
            ShareServer(tradePages: [1], proposal: .openNVDA(now: now)), subject: .passedProposal(id: "proposal-1"))
        await open.load()
        XCTAssertEqual(open.spec?.replayTitle, "Replay the price")
        XCTAssertEqual(open.title, "Share vote")
    }

    func testAnExpiredProposalHasNothingToShare() async {
        var expired = Components.Schemas.ProposalDetail.openNVDA(now: now)
        expired.status = .expired
        let model = make(ShareServer(tradePages: [1], proposal: expired), subject: .passedProposal(id: "proposal-1"))

        await model.load()

        guard case .failed = model.state else { return XCTFail("got \(model.state)") }
    }

    func testAnEyeFlipOnShareBRebuildsWithoutAnotherRead() async throws {
        let server = ShareServer(tradePages: [1], proposal: .filledMETA(now: now))
        let model = make(server, subject: .passedProposal(id: "proposal-1"))
        await model.load()
        let before = await server.paths.count

        model.eyes.pnlDollars = true

        XCTAssertEqual(model.spec?.figure.main, "+$3.31")
        let after = await server.paths.count
        XCTAssertEqual(after, before)
    }

    func testAStakeLoadsShareCFromThePotAndTheStakeHistory() async throws {
        let server = ShareServer(tradePages: [1])
        let model = make(server, subject: .stake(cabalID: "cabal-1"))

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.header.title, "Andres")
        XCTAssertEqual(spec.header.subtitle, "100% of \(Components.Schemas.Cabal.sample(role: "member").name)")
        XCTAssertEqual(spec.header.dateText, "Since Aug 2026")
        XCTAssertEqual(spec.chart.points.map(\.priceMicros), [7_000_000_000, 8_000_000_000])
        XCTAssertEqual(spec.replayTitle, "Replay your share")
        XCTAssertEqual(model.shareURL?.query, "c=cabal-1")
        let paths = await server.paths
        XCTAssertEqual(
            Set(paths),
            [
                "/v1/cabals/cabal-1/pot", "/v1/cabals/cabal-1", "/v1/me", "/v1/me/pnl-history",
                "/v1/me/referral-code",
            ])
    }

    func testAStakeInACabalYouAreNotInIsTheFailedState() async {
        let model = make(ShareServer(tradePages: [1], noSlice: true), subject: .stake(cabalID: "cabal-1"))

        await model.load()

        guard case .failed = model.state else { return XCTFail("got \(model.state)") }
    }

    func testThePortfolioLoadsTheMonthFigureAndRebasesTheChart() async throws {
        let server = ShareServer(tradePages: [1])
        let model = make(server, subject: .portfolio)

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.header.subtitle, "1 cabal")
        XCTAssertEqual(spec.figure.main, "+$41.25")
        XCTAssertEqual(spec.stats.map(\.value), ["$1,000", "+8.2%", "7 of 9"])
        XCTAssertEqual(spec.chart.points.map(\.priceMicros), [0, 40_000_000])
        XCTAssertEqual(spec.replayTitle, "Replay your month")
        let paths = await server.paths
        XCTAssertEqual(
            Set(paths),
            ["/v1/me/portfolio", "/v1/me/pnl-history", "/v1/me/referral-code", "/v1/me", "/v1/users/me-1"])
    }

    func testTheLedgerLoadsTheWeekTheViewerAndTheCode() async throws {
        let server = ShareServer(tradePages: [1])
        let model = make(server, subject: .ledger(week: "2026-W42"))
        XCTAssertEqual(model.title, "Week 42")

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.header.title, "Andres \u{00B7} Week 42")
        XCTAssertEqual(spec.figure.main, "+1.4%")
        XCTAssertEqual(model.title, "Week 42")
        XCTAssertNotNil(model.shareURL)
        model.format = .sticker
        XCTAssertEqual(model.spec?.figure.main, "+1.4%")
        let paths = await server.paths
        XCTAssertEqual(Set(paths), ["/v1/me/ledgers/2026-W42", "/v1/me", "/v1/me/referral-code"])
    }

    func testALedgerThatIsNotReadyIsTheFailedState() async {
        let model = make(ShareServer(tradePages: [1]), subject: .ledger(week: "2026-W01"))

        await model.load()

        guard case .failed = model.state else { return XCTFail("got \(model.state)") }
        XCTAssertEqual(model.title, "Week 1")
    }

    func testAPortfolioWithoutAMonthFigureIsTheFailedState() async {
        let model = make(ShareServer(tradePages: [1], noMonth: true), subject: .portfolio)

        await model.load()

        guard case .failed = model.state else { return XCTFail("got \(model.state)") }
    }

    func testTheStickerFormatTitlesTheScreenAndClearsTheEyesThenRestoresThem() async throws {
        let model = make(ShareServer(tradePages: [1]), subject: .stake(cabalID: "cabal-1"))
        await model.load()

        model.format = .sticker

        XCTAssertEqual(model.title, "Sticker")
        XCTAssertEqual(model.eyes, ShareScreenModel.stickerEyes)
        XCTAssertFalse(model.isVote)

        model.format = .story

        XCTAssertEqual(model.title, "Share")
        XCTAssertEqual(model.eyes, ShareEyes())
    }

    func testSwitchingBetweenOtherFormatsKeepsTheEyes() async {
        let model = make(ShareServer(tradePages: [1]), subject: .portfolio)
        model.eyes.code = false

        model.format = .square

        XCTAssertEqual(model.eyes, ShareEyes(code: false))
    }

    private func make(_ server: ShareServer, subject: ShareSubject? = nil, maxPages: Int = 20) -> ShareScreenModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: server)
        let now = now
        return ShareScreenModel(
            subject: subject ?? self.subject, api: api, now: { now }, timeZone: TimeZone(identifier: "UTC") ?? .gmt,
            maxPages: maxPages)
    }
}

private actor ShareServer: ClientTransport {
    private let tradePages: [Int]
    private let endless: Bool
    private let failing: String?
    private let locked: Bool
    private let proposal: Components.Schemas.ProposalDetail?
    private let noSlice: Bool
    private let noMonth: Bool
    private(set) var paths: [String] = []
    private(set) var chartQueries: [String] = []
    private(set) var activityCursors: [String?] = []

    init(
        tradePages: [Int], endless: Bool = false, failing: String? = nil, locked: Bool = false,
        proposal: Components.Schemas.ProposalDetail? = nil, noSlice: Bool = false, noMonth: Bool = false
    ) {
        self.noSlice = noSlice
        self.noMonth = noMonth
        self.proposal = proposal
        self.tradePages = tradePages
        self.endless = endless
        self.failing = failing
        self.locked = locked
    }

    func send(_ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String) async throws
        -> (HTTPResponse, HTTPBody?)
    {
        let components = URLComponents(string: request.path ?? "")
        let path = components?.path ?? ""
        paths.append(path)
        if path == failing { return reply(.internalServerError, "{}") }
        switch path {
        case "/v1/cabals/cabal-1/pot":
            guard noSlice else { return reply(.ok, Self.pot) }
            let bare = Self.pot.components(separatedBy: ",\"me\":").first ?? ""
            return reply(.ok, bare + ",\"me\":null}")
        case "/v1/proposals/proposal-1":
            guard let proposal else { return reply(.notFound, "{}") }
            return reply(.ok, try encode(proposal))
        case "/v1/cabals/\(Components.Schemas.Cabal.sample(role: nil).id)":
            return reply(.ok, try encode(Components.Schemas.Cabal.sample(role: "member")))
        case "/v1/assets/NVDAx/chart", "/v1/assets/METAx/chart":
            chartQueries.append(components?.query ?? "")
            return reply(.ok, try encode(Components.Schemas.AssetChart.oneDay))
        case "/v1/me/referral-code":
            return reply(
                .ok,
                try encode(locked ? Components.Schemas.MyReferralCode.sampleLocked : .sample))
        case "/v1/cabals/cabal-1/activity":
            let cursor = components?.queryItems?.first { $0.name == "cursor" }?.value
            activityCursors.append(cursor)
            return reply(.ok, activityPage(cursor: cursor))
        default: return try accountReply(path, components)
        }
    }

    private func accountReply(_ path: String, _ components: URLComponents?) throws -> (HTTPResponse, HTTPBody?) {
        switch path {
        case "/v1/me": return reply(.ok, Self.me)
        case "/v1/me/ledgers/2026-W42": return reply(.ok, try encode(Components.Schemas.Ledger.week42))
        case "/v1/users/me-1": return reply(.ok, noMonth ? Self.user(month: false) : Self.user(month: true))
        case "/v1/me/portfolio": return reply(.ok, Self.portfolio)
        case "/v1/me/pnl-history":
            let stake = components?.queryItems?.contains { $0.name == "cabal_id" } == true
            return reply(.ok, stake ? Self.stakeHistory : Self.monthHistory)
        case "/v1/cabals/cabal-1": return reply(.ok, try encode(Components.Schemas.Cabal.sample(role: "member")))
        default: return reply(.notFound, "{}")
        }
    }

    private func activityPage(cursor: String?) -> String {
        let index = cursor.flatMap { Int($0.dropFirst("page-".count)) } ?? 0
        let count = endless ? 1 : tradePages[index]
        let more = endless || index + 1 < tradePages.count
        let rows = (0..<count).map { row in
            let when = Date(timeIntervalSince1970: 1_780_000_000 - Double(index * 1_000 + row) * 3_600)
            return """
                {"id":"a-\(index)-\(row)","kind":"\(row % 2 == 0 ? "buy" : "sell")","status":"confirmed",\
                "asset":{"symbol":"NVDAx","name":"NVIDIA"},"usdc_micros":190000000,"units":100000000,\
                "price_micros":190000000,"actor":null,"tx_signature":null,\
                "occurred_at":"\(SharedFormatters.iso8601WholeSeconds.string(from: when))"}
                """
        }
        let next = more ? "\"page-\(index + 1)\"" : "null"
        return "{\"items\":[\(rows.joined(separator: ","))],\"next_cursor\":\(next)}"
    }

    private func reply(_ status: HTTPResponse.Status, _ body: String) -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: status)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(body))
    }

    private func encode<T: Encodable>(_ value: T) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private static let me = """
        {"id":"me-1","display_name":"Andres","share_performance":true,"auth_state":"ONBOARDING_COMPLETED",\
        "account_status":"active","login_provider":"sms","member_wallet_address":"w","phone_linked":true,\
        "created_at":"2026-08-01T12:00:00Z"}
        """

    private static func user(month: Bool) -> String {
        let figures =
            month
            ? "\"return_1m_bps\":820,\"pnl_1m_micros\":41250000"
            : "\"return_1m_bps\":null,\"pnl_1m_micros\":null"
        return """
            {"id":"me-1","handle":"andres","display_name":"Andres","photo_url":null,"bio":null,\
            "created_at":"2026-08-01T12:00:00Z","cabal_count":1,"follower_count":0,"following_count":0,\
            "followed_by_me":false,"blocked_by_me":false,\(figures),"return_period":"1M","return_private":false,\
            "standing":{"period":"month","period_start":"2026-10-01","rate_bps":7800,"right":7,"resolved":9,\
            "rank":8,"ranked_count":24,"more_to_rank":0},"crown":false,"cabals_in_common":0}
            """
    }

    private static let portfolio = """
        {"cash_micros":0,"net_worth_micros":1000000000,"total_value_micros":1000000000,"pnl_micros":0,\
        "return_bps":0,"computed_at":null,"prices_as_of":null,"cabals":[{"cabal":{"id":"cabal-1","name":"AC",\
        "picture_url":null},"value_micros":1000000000,"share_units":1,"net_contributed_micros":1000000000,\
        "pnl_micros":0,"return_bps":0,"slice_bps":10000,"ownership_bps":10000,"role":"member","can_vote":true,\
        "pot_value_micros":1000000000,"member_count":1,"open_vote_count":0}]}
        """

    private static let stakeHistory = """
        {"range":"ALL","pnl_micros":0,"return_bps":0,"points":[{"at":"2026-07-01T12:00:00Z","equity_micros":1,"pnl_micros":0},\
        {"at":"2026-08-19T15:00:00Z","equity_micros":7000000000,"pnl_micros":0},\
        {"at":"2026-10-01T12:00:00Z","equity_micros":8000000000,"pnl_micros":0}]}
        """

    private static let monthHistory = """
        {"range":"1M","pnl_micros":40000000,"return_bps":0,"points":[{"at":"2026-09-22T12:00:00Z","equity_micros":1,"pnl_micros":900000000},\
        {"at":"2026-10-22T12:00:00Z","equity_micros":1,"pnl_micros":940000000}]}
        """

    private static let pot = """
        {"cabal_id":"cabal-1","pot_value_micros":8360000000,"cash_micros":0,"cash_weight_bps":0,\
        "pnl_micros":1027900000,"return_bps":1402,"prices_as_of":"2026-10-22T15:00:00Z","holdings":[\
        {"symbol":"NVDAx","display_name":"NVIDIA","kind":"equity","units":"38.0000","token_amount":3800000000,\
        "price_micros":220000000,"value_micros":8360000000,"weight_bps":10000,"cost_basis_micros":7332100000,\
        "pnl_micros":1027900000,"avg_entry_micros":"192950000","first_bought_at":"2026-08-20T15:00:00Z"}],\
        "me":{"share_units":100000000,"value_micros":8360000000,"slice_bps":10000,\
        "net_contributed_micros":7332100000,"pnl_micros":1027900000,"first_funded_at":"2026-08-19T15:00:00Z"}}
        """
}
