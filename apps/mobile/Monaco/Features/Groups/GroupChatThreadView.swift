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

    @State private var tracker = ChatScrollTracker()
    @State private var scrollToBottomRequests = 0
    @State private var rowToKeepInView: String?
    @State private var topRowBeforeLoadingOlder: String?

    private static let bottomAnchor = "chat-bottom"

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 3) {
                    if hasOlder { loadEarlierButton }
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
                    Color.clear.frame(height: 1).id(Self.bottomAnchor)
                }
                .padding(.horizontal, 16)
                .padding(.bottom, 12)
            }
            .defaultScrollAnchor(.bottom, for: .initialOffset)
            .scrollDismissesKeyboard(.interactively)
            .refreshable { await refresh() }
            .onScrollGeometryChange(for: ChatScrollTracker.Position.self, of: Self.position) { _, updated in
                tracker.positionChanged(updated)
            }
            .onScrollGeometryChange(for: CGFloat.self, of: { $0.contentSize.height }) { _, _ in
                guard tracker.isFollowingThread else { return }
                proxy.scrollTo(Self.bottomAnchor, anchor: .bottom)
            }
            .onScrollPhaseChange { _, phase in track(phase) }
            .onChange(of: rows) { old, new in rowsChanged(from: old, to: new) }
            .onChange(of: isLoadingOlder) { _, loading in
                if !loading { topRowBeforeLoadingOlder = nil }
            }
            .onChange(of: scrollToBottomRequests) { _, _ in
                withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo(Self.bottomAnchor, anchor: .bottom) }
            }
            .onChange(of: rowToKeepInView) { _, rowID in
                guard let rowID else { return }
                proxy.scrollTo(rowID, anchor: .top)
                rowToKeepInView = nil
            }
            .accessibilityIdentifier("chat-thread")
            .overlay(alignment: .bottom) {
                if tracker.unreadCount > 0 { newMessagesPill }
            }
        }
    }

    private static func position(_ geometry: ScrollGeometry) -> ChatScrollTracker.Position {
        let bottom = geometry.contentOffset.y + geometry.containerSize.height
        return .init(
            offset: geometry.contentOffset.y,
            isAtEnd: bottom >= geometry.contentSize.height - ChatScrollTracker.pinnedSlack
        )
    }

    private func track(_ phase: ScrollPhase) {
        switch phase {
        case .interacting: tracker.dragBegan()
        case .idle: tracker.scrollSettled()
        default: return
        }
    }

    private func rowsChanged(from old: [ChatRow], to new: [ChatRow]) {
        if let kept = topRowBeforeLoadingOlder, new.first?.id != kept {
            rowToKeepInView = kept
            topRowBeforeLoadingOlder = nil
        }
        if tracker.arrived(ChatArrivals.added(from: old, to: new)) { scrollToBottomRequests += 1 }
    }

    private var loadEarlierButton: some View {
        Button {
            topRowBeforeLoadingOlder = rows.first?.id
            tracker.historyRequested()
            loadOlder()
        } label: {
            if isLoadingOlder {
                ProgressView().tint(MonacoTheme.accent)
            } else {
                Text(GroupChatCopy.loadEarlier).font(MonacoTheme.Typo.captionStrong)
            }
        }
        .buttonStyle(.borderless)
        .foregroundStyle(MonacoTheme.accent)
        .frame(maxWidth: .infinity)
        .padding(.vertical, 8)
        .disabled(isLoadingOlder)
        .accessibilityIdentifier("chat-load-earlier")
    }

    private var newMessagesPill: some View {
        Button {
            tracker.followRequested()
            scrollToBottomRequests += 1
        } label: {
            HStack(spacing: 6) {
                Image(systemName: "arrow.down").font(.caption.weight(.bold))
                Text(GroupChatCopy.newMessagesPill(count: tracker.unreadCount))
                    .font(MonacoTheme.Typo.captionStrong)
            }
            .foregroundStyle(MonacoTheme.primaryButtonLabel)
            .padding(.horizontal, 14)
            .padding(.vertical, 9)
            .background(Capsule().fill(MonacoTheme.primaryButtonFill))
        }
        .buttonStyle(.plain)
        .padding(.bottom, 10)
        .transition(.move(edge: .bottom).combined(with: .opacity))
        .animation(.snappy, value: tracker.unreadCount)
        .accessibilityIdentifier("chat-new-messages")
    }
}
