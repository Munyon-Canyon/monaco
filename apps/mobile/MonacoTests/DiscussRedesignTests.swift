import Foundation
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

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
