import Foundation
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct CommentThreadLayoutTests {
    @Test func eachLevelStepsInByOneIndent() {
        #expect(CommentThreadLayout.avatarLeading(level: 0) == MonacoTheme.Space.gutter)
        #expect(
            CommentThreadLayout.avatarLeading(level: 1)
                == MonacoTheme.Space.gutter + CommentThreadLayout.indentWidth)
        #expect(
            CommentThreadLayout.textLeading(level: 0) == MonacoTheme.Space.gutter + CommentThreadLayout.avatarSize
                + MonacoTheme.Space.sm)
    }

    @Test func replyGivesBackOnlyTheSpaceItsWordLeaves() {
        #expect(CommentRowMetrics.replyOverhang(lineHeight: 18) == 13)
        #expect(CommentRowMetrics.replyOverhang(lineHeight: 44) == 0)
        #expect(CommentRowMetrics.replyOverhang(lineHeight: 60) == 0)
    }
}
