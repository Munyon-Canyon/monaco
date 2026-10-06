import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct CommentThreadLayoutTests {
    private typealias Thread = Components.Schemas.CommentThread

    private func rows() -> [CommentThreadRow] {
        CommentThreadRow.rows(from: [
            Thread(comment: .sample("a"), replies: [.sample("b", parent: "a"), .sample("c", parent: "a")]),
            Thread(comment: .sample("d"), replies: []),
        ])
    }

    @Test func eachLevelStepsInByOneIndent() {
        #expect(CommentThreadLayout.avatarLeading(level: 0) == MonacoTheme.Space.m)
        #expect(CommentThreadLayout.avatarLeading(level: 1) == MonacoTheme.Space.m + CommentThreadLayout.indentWidth)
        #expect(
            CommentThreadLayout.textLeading(level: 0) == MonacoTheme.Space.m + CommentThreadLayout.avatarSize
                + MonacoTheme.Space.sm)
    }

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
        let rows = rows()
        #expect(rows.map(\.indentLevel) == [0, 1, 1, 0])
        #expect(CommentThreadLayout.hasReplies(rows, at: 0))
        #expect(!CommentThreadLayout.hasReplies(rows, at: 1))
        #expect(!CommentThreadLayout.hasReplies(rows, at: 2))
        #expect(!CommentThreadLayout.hasReplies(rows, at: 3))
        #expect(!CommentThreadLayout.hasReplies(rows, at: 9))
    }

    @Test func replyGivesBackOnlyTheSpaceItsWordLeaves() {
        #expect(CommentRowMetrics.replyOverhang(lineHeight: 18) == 13)
        #expect(CommentRowMetrics.replyOverhang(lineHeight: 44) == 0)
        #expect(CommentRowMetrics.replyOverhang(lineHeight: 60) == 0)
    }
}
