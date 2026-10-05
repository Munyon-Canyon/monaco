import Foundation
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

private func ballot(_ id: String, _ name: String, _ choice: String, castAt: String? = nil) -> ProposalVoteDTO {
    ProposalVoteDTO(voterId: id, displayName: name, choice: choice, castAt: castAt)
}

/// "Ada and Ben voted yes": who is behind a proposal, in the words friends use.
@MainActor
struct ProposalYesVotersTests {
    @Test func namesTheYesVotersInTheOrderTheServerSent() {
        let votes = [ballot("a", "Ada Park", "yes"), ballot("c", "Cy Lin", "no"), ballot("b", "Ben Ortiz", "YES")]
        #expect(ProposalYesVoters.names(in: votes, excluding: nil) == ["Ada Park", "Ben Ortiz"])
    }

    /// The viewer's vote has its own line on the card; naming them again would say it twice.
    @Test func leavesTheViewerOut() {
        let votes = [ballot("viewer", "Logan Norman", "yes"), ballot("a", "Ada Park", "yes")]
        #expect(ProposalYesVoters.names(in: votes, excluding: "viewer") == ["Ada Park"])
        #expect(ProposalYesVoters.names(in: votes, excluding: nil) == ["Logan Norman", "Ada Park"])
    }

    @Test func blankNamesAreNotVoters() {
        let votes = [ballot("a", "  ", "yes"), ballot("b", "Ben Ortiz", "yes")]
        #expect(ProposalYesVoters.names(in: votes, excluding: nil) == ["Ben Ortiz"])
    }

    @Test func nobodyYetSaysNothing() {
        #expect(ProposalYesVoters.sentence([]) == nil)
    }

    @Test func firstNamesJoinedTheWayPeopleSayThem() {
        #expect(ProposalYesVoters.sentence(["Ada Park"]) == "Ada voted yes")
        #expect(ProposalYesVoters.sentence(["Ada Park", "Ben Ortiz"]) == "Ada and Ben voted yes")
        #expect(ProposalYesVoters.sentence(["Ada Park", "Ben Ortiz", "Cy Lin"]) == "Ada, Ben and Cy voted yes")
    }

    @Test func pastThreeNamesTheRestAreCounted() {
        #expect(
            ProposalYesVoters.sentence(["Ada Park", "Ben Ortiz", "Cy Lin", "Dee Shah"])
                == "Ada, Ben, Cy and 1 other voted yes")
        #expect(
            ProposalYesVoters.sentence(["Ada Park", "Ben Ortiz", "Cy Lin", "Dee Shah", "Mia"])
                == "Ada, Ben, Cy and 2 others voted yes")
    }

    /// Two Adas would make "Ada and Ada voted yes", which says nothing about who.
    @Test func aSharedFirstNameKeepsItsSurname() {
        #expect(
            ProposalYesVoters.sentence(["Ada Park", "Ada Lin", "Ben Ortiz"]) == "Ada Park, Ada Lin and Ben voted yes")
    }

    @Test func aOneWordNameIsItsOwnFirstName() {
        #expect(ProposalYesVoters.sentence(["Mia"]) == "Mia voted yes")
    }

    @Test func theSentencesPassTheCopyAudit() {
        let sentences = [
            ProposalYesVoters.sentence(["Ada Park"]),
            ProposalYesVoters.sentence(["Ada Park", "Ben Ortiz", "Cy Lin", "Dee Shah", "Mia"]),
        ].compactMap { $0 }
        #expect(MainFlowCopyAudit.stringsAreClean(sentences))
    }
}

/// The card's corner: how long the vote has left while it is open, the one chip once it is not.
@MainActor
struct ProposalCardCornerTests {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)
    private let iso = ISO8601DateFormatter()

    private func proposal(status: String, expiresIn seconds: TimeInterval?, execution: String? = nil) -> ProposalDTO {
        ProposalDTO(
            id: "p",
            symbol: "AAPLx",
            status: status,
            kind: "buy",
            expiresAt: seconds.map { iso.string(from: now.addingTimeInterval($0)) },
            execution: execution.map { ProposalExecutionDTO(state: $0) }
        )
    }

    @Test func anOpenVoteShowsItsCountdown() {
        #expect(
            ProposalCardCorner.of(proposal(status: "open", expiresIn: 20 * 3600 + 60), now: now)
                == .countdown("Closes in 20h", closesSoon: false))
    }

    @Test func theLastHourIsFlagged() {
        #expect(
            ProposalCardCorner.of(proposal(status: "open", expiresIn: 30 * 60), now: now)
                == .countdown("Closes in 30m", closesSoon: true))
    }

    @Test func aClosedVoteShowsOneChipAndNoCountdown() {
        #expect(ProposalCardCorner.of(proposal(status: "failed", expiresIn: 3600), now: now) == .chip("Didn't pass"))
        #expect(
            ProposalCardCorner.of(proposal(status: "passed", expiresIn: nil, execution: "confirmed"), now: now)
                == .chip("Bought"))
        #expect(
            ProposalCardCorner.of(proposal(status: "passed", expiresIn: nil, execution: "pending"), now: now)
                == .chip("Buying"))
    }

    @Test func anOpenVoteWithNoReadableDeadlineShowsNothing() {
        #expect(ProposalCardCorner.of(proposal(status: "open", expiresIn: nil), now: now) == .none)
    }
}

/// Where a comment's face, words and rules go for how deep in the thread it sits.
@MainActor
struct CommentThreadLayoutTests {
    private func comment(_ id: String, parent: String? = nil) -> ProposalCommentDTO {
        ProposalCommentDTO(
            id: id, proposalId: "p", parentId: parent, authorId: "a", authorName: "Ada Park", body: "Why?",
            createdAt: "2026-09-24T10:00:00Z")
    }

    @Test func eachLevelStepsInByOneIndent() {
        #expect(CommentThreadLayout.avatarLeading(level: 0) == MonacoTheme.Space.m)
        #expect(CommentThreadLayout.avatarLeading(level: 1) == MonacoTheme.Space.m + CommentThreadLayout.indentWidth)
        #expect(
            CommentThreadLayout.textLeading(level: 0) == MonacoTheme.Space.m + CommentThreadLayout.avatarSize
                + MonacoTheme.Space.sm)
    }

    /// A reply's rules hang under the middle of each ancestor's face.
    @Test func threadRulesSitUnderTheAncestorsFaces() {
        #expect(CommentThreadLayout.threadRuleOffsets(level: 0).isEmpty)
        let offsets = CommentThreadLayout.threadRuleOffsets(level: 2)
        #expect(
            offsets == [
                CommentThreadLayout.avatarLeading(level: 0) + CommentThreadLayout.avatarSize / 2,
                CommentThreadLayout.avatarLeading(level: 1) + CommentThreadLayout.avatarSize / 2,
            ])
    }

    @Test func aRowStartsARuleOnlyWhenTheNextRowAnswersIt() {
        let rows = ProposalCommentThread.rows(from: [
            comment("a"), comment("b", parent: "a"), comment("c", parent: "a"), comment("d"),
        ])
        #expect(rows.map(\.depth) == [0, 1, 1, 0])
        #expect(CommentThreadLayout.hasReplies(rows, at: 0))
        #expect(!CommentThreadLayout.hasReplies(rows, at: 1))
        #expect(!CommentThreadLayout.hasReplies(rows, at: 2))
        #expect(!CommentThreadLayout.hasReplies(rows, at: 3))
        #expect(!CommentThreadLayout.hasReplies(rows, at: 9))
    }

    /// Reply keeps a 44pt target; the row only gives back what its word does not use, and
    /// nothing once the word is as tall as the target.
    @Test func replyGivesBackOnlyTheSpaceItsWordLeaves() {
        #expect(CommentRowMetrics.replyOverhang(lineHeight: 18) == 13)
        #expect(CommentRowMetrics.replyOverhang(lineHeight: 44) == 0)
        #expect(CommentRowMetrics.replyOverhang(lineHeight: 60) == 0)
    }

    /// Past the deepest indent a reply sits level with its parent, so no rule leads into it.
    @Test func pastTheDeepestIndentNoRuleLeadsOn() {
        let chain = (0...ProposalCommentThread.maxIndentLevel + 1).map { depth in
            comment("c\(depth)", parent: depth == 0 ? nil : "c\(depth - 1)")
        }
        let rows = ProposalCommentThread.rows(from: chain)
        let deepest = ProposalCommentThread.maxIndentLevel
        #expect(CommentThreadLayout.hasReplies(rows, at: deepest - 1))
        #expect(!CommentThreadLayout.hasReplies(rows, at: deepest))
    }
}
