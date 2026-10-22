import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ShareCardModelTests: XCTestCase {
    private typealias Cabal = Components.Schemas.Cabal

    private static let historyPath = "/v1/cabals/cabal-1/value-history?range=ALL"
    private static let codePath = "/v1/me/referral-code"
    private static let potPath = "/v1/cabals/cabal-1/pot"
    private static let meHistoryPath = "/v1/me/pnl-history?range=ALL"
    private static let portfolioPath = "/v1/me/portfolio"

    func testTheCabalCardLoadsWithTheHandleLink() async throws {
        let (model, transport, _) = try make([
            Self.historyPath: [try json(Components.Schemas.CabalValueHistory.sample())],
            Self.codePath: [try json(Components.Schemas.MyReferralCode.sample)],
        ])

        await model.loadCabal(cabalID: "cabal-1", cabal: Cabal.sampleWithMembers(role: "member"), pot: .sampleInvested)

        let content = try XCTUnwrap(model.content)
        XCTAssertEqual(content.footerLink, "monacolabs.xyz/r/kaicenat")
        XCTAssertEqual(content.gain, "+$0.73")
        let paths = await transport.sent.compactMap(\.path).sorted()
        XCTAssertEqual(paths, [Self.historyPath, Self.codePath].sorted())
    }

    func testAPendingCodeWaitsOnceThenFallsBackToTheSite() async throws {
        let (model, transport, sleeps) = try make([
            Self.historyPath: [try json(Components.Schemas.CabalValueHistory.sample())],
            Self.codePath: [try Self.pending(), try Self.pending()],
        ])

        await model.loadCabal(cabalID: "cabal-1", cabal: Cabal.sampleWithMembers(role: "member"), pot: .sampleInvested)

        let content = try XCTUnwrap(model.content)
        XCTAssertEqual(content.footerLink, "monacolabs.xyz")
        XCTAssertEqual(content.shareURL, ShareCardContent.fallbackURL)
        let waits = await sleeps.durations
        XCTAssertEqual(waits, [InviteLinkModel.pendingRetryDelay])
        let codeReads = await transport.sent.filter { $0.path == Self.codePath }.count
        XCTAssertEqual(codeReads, 2)
    }

    func testAPendingCodeThatLandsOnTheRetryIsUsed() async throws {
        let (model, _, _) = try make([
            Self.historyPath: [try json(Components.Schemas.CabalValueHistory.sample())],
            Self.codePath: [try Self.pending(), try json(Components.Schemas.MyReferralCode.sampleLocked)],
        ])

        await model.loadCabal(cabalID: "cabal-1", cabal: Cabal.sampleWithMembers(role: "member"), pot: .sampleInvested)

        XCTAssertEqual(model.content?.footerLink, "monacolabs.xyz/r/k7m4qx2p")
    }

    func testAFailingCodeReadStillLoadsTheCard() async throws {
        let (model, _, _) = try make([
            Self.historyPath: [try json(Components.Schemas.CabalValueHistory.sample())],
            Self.codePath: [.failure(URLError(.notConnectedToInternet))],
        ])

        await model.loadCabal(cabalID: "cabal-1", cabal: Cabal.sampleWithMembers(role: "member"), pot: .sampleInvested)

        XCTAssertEqual(model.content?.footerLink, "monacolabs.xyz")
    }

    func testAFailingHistoryIsTheFailedState() async throws {
        let (model, _, _) = try make([
            Self.historyPath: [.failure(URLError(.notConnectedToInternet))],
            Self.codePath: [try json(Components.Schemas.MyReferralCode.sample)],
        ])

        await model.loadCabal(cabalID: "cabal-1", cabal: Cabal.sampleWithMembers(role: "member"), pot: .sampleInvested)

        guard case .failed(.transport) = model.state else { return XCTFail("want a transport failure") }
        XCTAssertNil(model.content)
    }

    func testTheCabalPotIsReadWhenTheScreenHasNone() async throws {
        let (model, transport, _) = try make([
            Self.historyPath: [try json(Components.Schemas.CabalValueHistory.sample())],
            Self.potPath: [try json(Components.Schemas.CabalPot.sampleInvested)],
            Self.codePath: [try json(Components.Schemas.MyReferralCode.sample)],
        ])

        await model.loadCabal(cabalID: "cabal-1", cabal: Cabal.sampleWithMembers(role: "member"))

        XCTAssertEqual(model.content?.gain, "+$0.73")
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertTrue(paths.contains(Self.potPath))
    }

    func testABannedCabalFailsInsteadOfDrawing() async throws {
        let (model, _, _) = try make([
            Self.historyPath: [try json(Components.Schemas.CabalValueHistory.sample())],
            Self.codePath: [try json(Components.Schemas.MyReferralCode.sample)],
        ])

        await model.loadCabal(cabalID: "cabal-1", cabal: Cabal.sampleBanned(role: "member"), pot: .sampleInvested)

        guard case .failed = model.state else { return XCTFail("want a failed state") }
    }

    func testMyCardLoadsFromThePortfolioAndHistory() async throws {
        let (model, transport, _) = try make([
            Self.meHistoryPath: [try json(Components.Schemas.MyPnlHistory.sample(range: .all))],
            Self.portfolioPath: [try json(Components.Schemas.MyPortfolio.sample)],
            Self.codePath: [try json(Components.Schemas.MyReferralCode.sample)],
        ])

        await model.loadMe(profile: SessionProfile(Components.Schemas.Me.sample))

        let content = try XCTUnwrap(model.content)
        XCTAssertEqual(content.kind, .me)
        XCTAssertEqual(content.gain, "+$14.00")
        let count = await transport.sent.count
        XCTAssertEqual(count, 3)
    }

    func testAPreviewModelIsAlreadyLoaded() async throws {
        let content = try XCTUnwrap(
            ShareCardContent.cabal(
                Cabal.sampleWithMembers(role: "member"), pot: .sampleInvested, history: .sample(), links: nil))

        XCTAssertEqual(ShareCardModel.preview(content).content, content)
    }

    private func make(
        _ routes: [String: [StubTransport.Reply]]
    ) throws -> (ShareCardModel, StubTransport, SleepLog) {
        let transport = StubTransport(routes: routes)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let sleeps = SleepLog()
        let model = ShareCardModel(
            api: api, locale: Locale(identifier: "en_US"), timeZone: TimeZone(identifier: "UTC") ?? .current,
            sleep: { await sleeps.record($0) })
        return (model, transport, sleeps)
    }

    private func json<T: Encodable>(_ value: T) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(value), as: UTF8.self))
    }

    private static func pending() throws -> StubTransport.Reply {
        try .problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: 503, code: .referralCodePending,
                message: "Your invite code is on its way.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
                retryable: true))
    }
}

private actor SleepLog {
    private(set) var durations: [Duration] = []

    func record(_ duration: Duration) {
        durations.append(duration)
    }
}
