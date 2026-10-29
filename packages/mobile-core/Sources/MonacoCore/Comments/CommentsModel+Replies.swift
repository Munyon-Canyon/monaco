import MonacoAPI

extension CommentsModel {
    public var visibleRows: [CommentThreadRow] {
        rows.filter { !$0.isFolded || expandedRoots.contains($0.comment.parentCommentId ?? "") }
    }

    public var entries: [CommentThreadEntry] {
        let all = rows
        var result: [CommentThreadEntry] = []
        for (index, row) in all.enumerated() {
            if row.isFolded {
                guard expandedRoots.contains(row.comment.parentCommentId ?? "") else { continue }
                result.append(.row(row))
                if index + 1 == all.count || !all[index + 1].isFolded, let rootID = row.comment.parentCommentId {
                    result.append(.fold(rootID: rootID, count: row.foldedCountOfRoot(in: all), isExpanded: true))
                }
                continue
            }
            result.append(.row(row))
            if row.foldedCount > 0, !expandedRoots.contains(row.id) {
                result.append(.fold(rootID: row.id, count: row.foldedCount, isExpanded: false))
            }
        }
        return result
    }

    public func isExpanded(rootID: String) -> Bool { expandedRoots.contains(rootID) }

    public func expand(rootID: String) {
        expandedRoots.insert(rootID)
    }

    public func collapse(rootID: String) {
        expandedRoots.remove(rootID)
    }

    public func jump(to quote: CommentQuoteRow) {
        guard quote.canJump, rows.contains(where: { $0.id == quote.targetID }) else { return }
        if quote.targetIsFolded { expandedRoots.insert(quote.targetRootID) }
        scrollRequest = ScrollRequest(id: quote.targetID, tick: (scrollRequest?.tick ?? 0) + 1)
        highlightedID = quote.targetID
        highlightTask?.cancel()
        let target = quote.targetID
        highlightTask = Task { [clock] in
            try? await clock.sleep(for: Self.highlightDuration)
            guard !Task.isCancelled else { return }
            if highlightedID == target { highlightedID = nil }
        }
    }

    public func reactions(for row: CommentThreadRow) -> [Components.Schemas.Reaction] {
        reactions?.reactions(for: .comment(row.id), fallback: row.reactions) ?? row.reactions
    }

    public var reactionNotice: String? { reactions?.notice }

    public var reactionNoticeTick: Int { reactions?.noticeTick ?? 0 }

    public func react(_ emoji: String, to row: CommentThreadRow) async {
        guard let reactions, row.canReact else { return }
        let current = reactions.reactions(for: .comment(row.id), fallback: row.reactions)
        let updated = await reactions.toggle(emoji, on: .comment(row.id), current: current)
        pager.update(id: row.comment.parentCommentId ?? row.id) { thread in
            if thread.comment.id == row.id { thread.comment.setReactions(updated) }
            for index in thread.replies.indices where thread.replies[index].id == row.id {
                thread.replies[index].setReactions(updated)
            }
        }
    }

    public func doubleTap(_ row: CommentThreadRow) async {
        guard row.canReact, !reactions(for: row).contains(where: { $0.emoji == Self.heart && $0.mine }) else { return }
        await react(Self.heart, to: row)
    }

    public static let heart = "❤️"

    public var commentCount: Int { rows.filter { !$0.isDeleted }.count }
}

extension CommentThreadRow {
    fileprivate func foldedCountOfRoot(in rows: [CommentThreadRow]) -> Int {
        rows.first { $0.id == comment.parentCommentId }?.foldedCount ?? 0
    }
}

extension Components.Schemas.Comment {
    fileprivate mutating func setReactions(_ updated: [Components.Schemas.Reaction]) {
        reactions = updated
        reactionCount = Int32(updated.reduce(0) { $0 + Int($1.count) })
    }
}
