import Foundation
import MonacoAPI
import Testing

@testable import Monaco
@testable import MonacoCore

@MainActor
struct ChatPinCapsuleTests {
    private let viewer = "u-me"

    private func message(
        _ id: String = "m1", author: String = "u-sam", name: String = "Sam", body: String? = "gm",
        pinnedBy: String? = nil
    ) throws -> ChatMessage {
        let base = ChatMessage(
            id: id, kind: .user,
            author: .init(id: author, handle: nil, displayName: name, photoUrl: nil),
            body: body, createdAt: Date(timeIntervalSince1970: 1_790_000_000),
            alsoInChannel: false, replyCount: 0, lastRepliers: [], deleted: false, reactions: [], reactionCount: 0)
        guard let pinnedBy else { return base }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var object = try #require(JSONSerialization.jsonObject(with: encoder.encode(base)) as? [String: Any])
        object["pinned_at"] = "2026-11-04T12:00:00Z"
        object["pinned_by"] = ["id": "u-pinner", "handle": NSNull(), "display_name": pinnedBy, "photo_url": NSNull()]
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(ChatMessage.self, from: JSONSerialization.data(withJSONObject: object))
    }

    private func controls(role: String?) -> ChatPinControls {
        let transport = StubTransport(.json(.ok, "{}"))
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return ChatPinControls(
            pins: ChatPinsModel(cabalID: "c1", viewerID: viewer, api: api), role: role, viewerID: viewer)
    }

    @Test func theCounterShowsOnlyWithTwoOrMorePins() {
        #expect(ChatPinCopy.counter(index: 0, count: 1) == nil)
        #expect(ChatPinCopy.counter(index: 0, count: 2) == "1 of 2")
        #expect(ChatPinCopy.counter(index: 2, count: 3) == "3 of 3")
    }

    @Test func thePinnerShowsByFirstName() throws {
        #expect(ChatPinCopy.pinner(of: try message(pinnedBy: "Christelle Dupont")) == "Christelle")
        #expect(ChatPinCopy.pinner(of: try message()) == "Someone")
        #expect(ChatPinCopy.firstName("  ") == "Someone")
    }

    @Test func theTextCollapsesWhitespace() throws {
        let text = ChatPinCopy.text(of: try message(body: "5 of 8\n to   pass"), cabalName: "AC", viewerID: viewer)
        #expect(text == "5 of 8 to pass")
    }

    @Test func voiceOverReadsThePinnerTheTextAndTheCounter() {
        #expect(
            ChatPinCopy.spokenLabel(pinner: "Christelle", text: "6 hour votes", counter: "1 of 2")
                == "Pinned by Christelle: 6 hour votes. 1 of 2.")
        #expect(
            ChatPinCopy.spokenLabel(pinner: "Christelle", text: "6 hour votes", counter: nil)
                == "Pinned by Christelle: 6 hour votes.")
    }

    @Test func adminsPinAnyMessageAndMembersOnlyTheirOwn() throws {
        let theirs = try message(author: "u-sam")
        let mine = try message(author: viewer, name: "Me")
        #expect(controls(role: "admin").canPin(theirs))
        #expect(controls(role: "member").canPin(mine))
        #expect(!controls(role: "member").canPin(theirs))
    }

    @Test func theCapsuleIsFortyPointsWithTargetsOfFortyFour() {
        #expect(ChatPinMetrics.height == 40)
        #expect(ChatPinMetrics.minTarget == 44)
        #expect(ChatPinMetrics.listRowLines == 3)
    }

    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private func summary(
        id: String = "p1", yes: Int = 4, no: Int = 1, replies: Int = 6, canVote: Bool = true,
        ballot: Components.Schemas.Proposal.MyBallotPayload? = nil
    ) -> ProposalSummary {
        var proposal = Components.Schemas.Proposal.sample()
        proposal.id = id
        proposal.symbol = "NVDAx"
        proposal.usdcMicros = 400_000_000
        proposal.tally = .init(yes: yes, no: no, voters: 8, needed: 5)
        proposal.replyCount = replies
        proposal.canVote = canVote
        proposal.myBallot = ballot
        proposal.expiresAt = now.addingTimeInterval(4 * 3_600)
        return ProposalSummary(proposal)
    }

    private func vote(_ summary: ProposalSummary) -> ChatPinVote {
        ChatPinVote(summary: summary, headline: "Buy NVDA \u{00B7} $400", asset: nil, now: now)
    }

    private func row(_ id: String, proposalID: String?) throws -> ChatRow {
        var message = try message(id)
        message.proposalId = proposalID
        return ChatRow(
            message: message, moment: nil, pendingAttachment: nil, parentBody: nil, delivery: .sent, isMine: false,
            startsDay: false, startsRun: true, endsRun: true)
    }

    @Test func theVoteCapsuleCarriesTheHeadlineTheTallyLineAndVote() {
        let pinned = vote(summary())
        #expect(pinned.headline == "Buy NVDA \u{00B7} $400")
        #expect(pinned.line == "5 of 8 voted \u{00B7} 6 replies \u{00B7} 4h")
        #expect(pinned.showsVote)
        #expect(ChatPinMetrics.voteHeight == 56)
    }

    @Test func voiceOverReadsTheOpenVote() {
        #expect(
            vote(summary()).spoken
                == "Open vote. Buy NVDA, 400 dollars. 5 of 8 voted, 6 replies, closes in 4 hours.")
    }

    @Test func voteHidesWhenTheViewerCannotVoteOrAlreadyVoted() {
        #expect(!vote(summary(canVote: false)).showsVote)
        let voted = vote(summary(canVote: false, ballot: .yes))
        #expect(!voted.showsVote)
        #expect(voted.line == "5 of 8 voted \u{00B7} 6 replies \u{00B7} 4h \u{00B7} you voted Yes")
        #expect(voted.spoken.hasSuffix("You voted Yes."))
        #expect(!vote(summary(ballot: .no)).showsVote)
    }

    @Test func theLineStaysShortWithOneReplyAndNone() {
        #expect(vote(summary(yes: 2, no: 0, replies: 1)).line == "2 of 8 voted \u{00B7} 1 reply \u{00B7} 4h")
        #expect(vote(summary(replies: 0)).line == "5 of 8 voted \u{00B7} 4h")
    }

    @Test func sliversShowWithMoreThanOneItemAndTheCounterStaysWithPins() {
        #expect(!ChatPinCopy.showsSlivers(itemCount: 0))
        #expect(!ChatPinCopy.showsSlivers(itemCount: 1))
        #expect(ChatPinCopy.showsSlivers(itemCount: 2))
        #expect(ChatPinMetrics.sliverDrops == [4, 8])
        #expect(ChatPinMetrics.sliverInsets == [8, 16])
        #expect(ChatPinCopy.counter(index: 0, count: 3) == "1 of 3")
    }

    @Test func tappingTheBodyScrollsToALoadedCardAndOtherwiseOpensTheProposal() throws {
        let rows = [try row("m1", proposalID: nil), try row("m2", proposalID: "p1")]
        #expect(ChatPinVoteTarget.resolve(proposalID: "p1", rows: rows) == .scroll("m2"))
        #expect(ChatPinVoteTarget.resolve(proposalID: "p2", rows: rows) == .proposal("p2"))
    }
}
