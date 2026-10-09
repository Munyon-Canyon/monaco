import MonacoAPI
import MonacoCore
import SwiftUI

struct GroupChatThreadView: View {
    let rows: [ChatRow]
    var members: [Components.Schemas.CabalMember] = []
    let hasOlder: Bool
    let isLoadingOlder: Bool
    let openProfile: (String) -> Void
    let openThread: (String) -> Void
    let requestDelete: (String) -> Void
    let seen: ChatSession.Seen?
    let openSeen: (String) -> Void
    let retry: (String) -> Void
    let loadOlder: () -> Void
    let refresh: () async -> Void

    var body: some View {
        ChatList(
            rows: rows, hasOlder: hasOlder, isLoadingOlder: isLoadingOlder,
            loadEarlierID: "chat-load-earlier", listID: "chat-thread",
            loadOlder: loadOlder, refresh: refresh
        ) {
            EmptyView()
        } content: {
            ForEach(rows) { row in
                GroupChatRowView(
                    row: row, now: Date(), members: members, openProfile: openProfile, retry: retry,
                    openThread: openThread, requestDelete: requestDelete,
                    seenLabel: row.id == seen?.messageID
                        ? seen.flatMap { ChatSeenCopy.label(count: $0.count) } : nil,
                    openSeen: { openSeen(row.id) }
                )
                .id(row.id)
            }
        }
    }
}
