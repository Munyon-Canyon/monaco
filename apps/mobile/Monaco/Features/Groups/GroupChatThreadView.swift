import MonacoAPI
import MonacoCore
import SwiftUI

struct GroupChatThreadView: View {
    let rows: [ChatRow]
    var members: [Components.Schemas.CabalMember] = []
    var cabalName = ""
    var proposals: ChatProposalStore?
    var openProposal: ((String) -> Void)?
    let hasOlder: Bool
    let isLoadingOlder: Bool
    let openProfile: (String) -> Void
    let openThread: (String) -> Void
    let requestDelete: (String) -> Void
    let seen: ChatSession.Seen?
    let openSeen: (String) -> Void
    let retry: (String) -> Void
    let discard: (String) -> Void
    let loadOlder: () -> Void
    let refresh: () async -> Void
    var reactions: ReactionsModel?
    var cabalID = ""
    var quiet: ChatQuietContent?
    var cardVisibility: (String, Bool) -> Void = { _, _ in }

    @Environment(ToastCenter.self) private var toasts
    @Environment(AppEnvironment.self) private var environment: AppEnvironment?

    @State private var now = Date()
    @State private var prices: ChatMomentPrices?

    var body: some View {
        ChatList(
            rows: rows, hasOlder: hasOlder, isLoadingOlder: isLoadingOlder,
            loadEarlierID: "chat-load-earlier", listID: "chat-thread",
            loadOlder: loadOlder, refresh: refresh,
            pinsToBottom: quiet?.showsActions == true
        ) {
            ChatQuietGroup(visible: quiet?.showsScene == true) {
                if let quiet { ChatQuietHeader(content: quiet) }
            }
        } content: {
            ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                GroupChatRowView(
                    row: row, now: now, members: members, cabalName: cabalName, proposals: proposals,
                    openProfile: openProfile, retry: retry, discard: discard, openThread: openThread,
                    openProposal: openProposal, requestDelete: requestDelete,
                    seenLabel: row.id == seen?.messageID
                        ? seen.flatMap { ChatSeenCopy.label(count: $0.count) } : nil,
                    openSeen: { openSeen(row.id) }, reactions: reactions, cabalID: cabalID,
                    momentPrice: row.moment?.symbol.flatMap { prices?.price(for: $0) },
                    followsEventLine: Self.followsEventLine(
                        previous: index > 0 ? rows[index - 1].moment : nil, current: row.moment),
                    openMoment: { openMoment($0) }
                )
                .id(row.id)
                .modifier(ChatCardVisibility(proposalID: row.message.proposalId, report: cardVisibility))
            }
            ChatQuietGroup(visible: quiet?.showsActions == true) {
                if let quiet { ChatQuietActions(content: quiet) }
            }
        }
        .task(id: ChatMomentPrices.symbols(in: rows)) { await refreshPrices() }
        .onChange(of: rows) { now = Date() }
        .onChange(of: reactions?.noticeTick) { _, _ in
            if let message = reactions?.notice { toasts.current = MonacoToast(message: message) }
        }
    }

    static func followsEventLine(previous: ChatMoment?, current: ChatMoment?) -> Bool {
        guard let previous, let current else { return false }
        return ChatMomentRow.isEventLine(previous) && ChatMomentRow.isEventLine(current)
    }

    private func refreshPrices() async {
        guard let api = environment?.api else { return }
        let model = prices ?? ChatMomentPrices(api: api)
        prices = model
        await model.refresh(rows: rows)
    }

    private func openMoment(_ moment: ChatMoment) {
        guard let environment, let destination = ChatMomentRow.destination(of: moment) else { return }
        let tab = environment.navigator.selectedTab
        switch destination {
        case .proposal(let id): environment.navigator.open(ProposalRoute(proposalID: id), in: tab)
        case .holding(let symbol):
            environment.navigator.open(CabalHoldingRoute(cabalID: cabalID, symbol: symbol), in: tab)
        }
    }
}

private struct ChatCardVisibility: ViewModifier {
    let proposalID: String?
    let report: (String, Bool) -> Void

    @ViewBuilder func body(content: Content) -> some View {
        if let proposalID {
            content
                .onScrollVisibilityChange(threshold: 0.2) { report(proposalID, $0) }
                .onDisappear { report(proposalID, false) }
        } else {
            content
        }
    }
}
