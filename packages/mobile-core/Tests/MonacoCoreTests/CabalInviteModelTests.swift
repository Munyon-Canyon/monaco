import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalInviteModelTests: XCTestCase {
    private static let cabalID = "01890a5d-ac96-774b-bcce-b302099a8058"
    private static let codePath = "/v1/me/referral-code"
    private static let previewPath = "/v1/referrals/kaicenat/cabals/\(cabalID)"
    private nonisolated static let expiry = Date(timeIntervalSince1970: 1_793_200_000)
    private nonisolated static let now = expiry.addingTimeInterval(-(4 * 3600 + 720))

    func testTheFrameReadsAsDrawn() async throws {
        let (model, _, _) = try make([.sample])
        await model.load()

        let preview = try XCTUnwrap(model.preview)
        XCTAssertEqual(preview.title, "Invite to AC Capital")
        XCTAssertEqual(preview.facts, "8 members \u{00B7} $20,000 treasury")
        XCTAssertEqual(preview.inviterLine, "Andres invited you")
        XCTAssertEqual(preview.faces.count, 3)
        XCTAssertEqual(preview.moreText, "+4")
        let proposal = try XCTUnwrap(preview.proposal)
        XCTAssertEqual(proposal.title, "Buy NVDA \u{00B7} $400")
        XCTAssertEqual(proposal.detail, "5 of 8 voted \u{00B7} 6 replies \u{00B7} ends in 4h")
        XCTAssertEqual(proposal.yes, 4)
        XCTAssertEqual(proposal.no, 1)
    }

    func testNoOpenProposalLeavesNoRow() async throws {
        let (model, _, _) = try make([.sampleNoOpenProposal])
        await model.load()

        XCTAssertNil(try XCTUnwrap(model.preview).proposal)
    }

    func testNullMonthFactsStillDrawTheCard() async throws {
        let (model, _, _) = try make([.sampleMonthFactsNull])
        await model.load()

        let preview = try XCTUnwrap(model.preview)
        XCTAssertEqual(preview.facts, "8 members \u{00B7} $20,000 treasury")
        XCTAssertNil(preview.proposal)
    }

    func testNullRepliesDropTheRepliesPart() async throws {
        var source = Components.Schemas.ReferralCabal.sample
        source.openProposal?.replies = nil
        let (model, _, _) = try make([source])
        await model.load()

        XCTAssertEqual(
            try XCTUnwrap(model.preview?.proposal).detail, "5 of 8 voted \u{00B7} ends in 4h")
    }

    func testOneReplyIsSingular() async {
        XCTAssertEqual(
            CabalInviteCopy.proposalDetail(voted: 2, eligible: 6, replies: 1, expiresAt: Self.expiry, now: Self.now),
            "2 of 6 voted \u{00B7} 1 reply \u{00B7} ends in 4h")
    }

    func testThePlusCountIsTheMembersNotShownBesideYou() async throws {
        let cases: [(members: Int32, faces: Int, more: String?)] = [
            (8, 4, "+3"), (8, 3, "+4"), (5, 4, nil), (1, 0, nil), (2, 1, nil), (3, 1, "+1"),
        ]
        for (members, faces, more) in cases {
            var source = Components.Schemas.ReferralCabal.sample
            source.cabal?.memberCount = members
            source.cabal?.firstMembers = Array(
                repeating: .init(displayName: "Mia", photoUrl: nil), count: faces)
            let preview = try XCTUnwrap(CabalInviteCopy.preview(source, now: Self.now))
            XCTAssertEqual(preview.moreText, more, "\(members) members, \(faces) faces")
        }
    }

    func testOnlyTheFirstNameIsUsedInTheInviterLine() async {
        XCTAssertEqual(CabalInviteCopy.inviterLine(displayName: "Andres Campos"), "Andres invited you")
    }

    func testAWholeDollarAmountDropsTheCents() async throws {
        var source = Components.Schemas.ReferralCabal.sample
        source.openProposal?.usdcMicros = "400500000"
        source.openProposal?.kind = "sell"
        let preview = try XCTUnwrap(CabalInviteCopy.preview(source, now: Self.now))
        XCTAssertEqual(preview.proposal?.title, "Sell NVDA \u{00B7} $400.50")
    }

    func testTimeLeftReadsInTheLargestUnit() async {
        let cases: [(TimeInterval, String)] = [
            (-5, "ends now"), (30, "ends in 1m"), (90, "ends in 1m"), (45 * 60, "ends in 45m"),
            (3600 * 5 + 59, "ends in 5h"), (3600 * 23, "ends in 23h"), (3600 * 49, "ends in 2d"),
        ]
        for (seconds, text) in cases {
            XCTAssertEqual(CabalInviteCopy.endsIn(Self.now.addingTimeInterval(seconds), now: Self.now), text)
        }
    }

    func testTheLinkCarriesTheCabal() async throws {
        let (model, _, _) = try make([.sample])
        await model.load()

        XCTAssertEqual(model.link?.absoluteString, "https://monacolabs.xyz/r/kaicenat?c=\(Self.cabalID)")
    }

    func testAnUnresolvedPreviewFails() async throws {
        let (model, _, _) = try make([.sampleMiss])
        await model.load()

        guard case .failed = model.state else { return XCTFail("state \(model.state)") }
    }

    func testAFailedReadKeepsTheLastPreview() async throws {
        let transport = StubTransport(routes: [
            Self.codePath: [.json(.ok, Self.code)],
            Self.previewPath: [try Self.reply(.sample), .failure(URLError(.notConnectedToInternet))],
        ])
        let model = makeModel(transport, hints: FakeHintStream())
        await model.load()
        await model.load()

        XCTAssertNotNil(model.preview)
    }

    func testEveryHintRereadsThePreview() async throws {
        let id = Self.cabalID
        let hints: [Hint] = [
            .changed(.cabal(id), what: "proposal_created", id: "1"),
            .changed(.cabal(id), what: "proposal_updated", id: "2"),
            .changed(.global, what: "feed", id: "3"),
            .changed(.cabal(id), what: "members", id: "4"),
            .resync,
        ]
        for hint in hints {
            let (model, transport, stream) = try make(Array(repeating: .sample, count: 4))
            await model.load()
            let observer = Task { await model.observe() }
            await stream.waitForSubscribers(4)

            await stream.send(hint)

            let reread = await waitUntil { await self.previewReads(transport) == 2 }
            XCTAssertTrue(reread, "\(hint)")
            observer.cancel()
        }
    }

    func testAnUnrelatedHintDoesNotReread() async throws {
        let (model, transport, stream) = try make(Array(repeating: .sample, count: 4))
        await model.load()
        let observer = Task { await model.observe() }
        await stream.waitForSubscribers(4)

        await stream.send(.changed(.cabal(Self.cabalID), what: "activity_changed", id: "1"))
        await stream.send(.changed(.cabal("another"), what: "proposal_updated", id: "2"))
        for _ in 0..<200 { await Task.yield() }

        let reads = await previewReads(transport)
        XCTAssertEqual(reads, 1)
        observer.cancel()
    }

    func testAVoteWhileTheSheetIsOpenMovesTheSplit() async throws {
        var after = Components.Schemas.ReferralCabal.sample
        after.openProposal?.yes = 5
        after.openProposal?.voted = 6
        let (model, _, stream) = try make([.sample, after])
        await model.load()
        XCTAssertEqual(model.preview?.proposal?.yes, 4)
        XCTAssertEqual(model.preview?.proposal?.no, 1)
        let observer = Task { await model.observe() }
        await stream.waitForSubscribers(4)

        await stream.send(.changed(.cabal(Self.cabalID), what: "proposal_updated", id: "1"))

        let moved = await waitUntil { model.preview?.proposal?.yes == 5 }
        XCTAssertTrue(moved)
        XCTAssertEqual(model.preview?.proposal?.yes, 5)
        XCTAssertEqual(model.preview?.proposal?.no, 1)
        XCTAssertEqual(model.preview?.proposal?.detail, "6 of 8 voted \u{00B7} 6 replies \u{00B7} ends in 4h")
        observer.cancel()
    }

    private static let code =
        #"{"code":"k7m4qx2p","link":"https://monacolabs.xyz/r/k7m4qx2p","#
        + #""handle_link":"https://monacolabs.xyz/r/kaicenat","handle_unlocked":true}"#

    private func make(
        _ previews: [Components.Schemas.ReferralCabal]
    ) throws -> (CabalInviteModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(routes: [
            Self.codePath: [.json(.ok, Self.code)],
            Self.previewPath: try previews.map { try Self.reply($0) },
        ])
        let hints = FakeHintStream()
        return (makeModel(transport, hints: hints), transport, hints)
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream) -> CabalInviteModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let links = InviteLinkModel(api: api, hints: hints)
        return CabalInviteModel(
            api: api, hints: hints, cabalID: Self.cabalID, inviteLinks: links, now: { Self.now })
    }

    private func previewReads(_ transport: StubTransport) async -> Int {
        await transport.sent.filter { $0.path == Self.previewPath }.count
    }

    private static func reply(_ source: Components.Schemas.ReferralCabal) throws -> StubTransport.Reply {
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        response.headerFields[.cacheControl] = "no-cache"
        return .response(response, Data(try json(source).utf8))
    }

    private static func json(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
