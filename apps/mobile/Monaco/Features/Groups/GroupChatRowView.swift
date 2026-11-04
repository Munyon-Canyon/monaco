import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatBubbleMetrics {
    static let avatarSize: CGFloat = 28
    static let avatarHitInset: CGFloat = -9
    static let widthFraction: CGFloat = 0.87
    static let mineWidthFraction: CGFloat = 0.78
    static let runGap: CGFloat = 2
    static let groupGap: CGFloat = 6
    static let nameInset: CGFloat = 12
    static let sentOpacity: Double = 1
    static let pendingOpacity: Double = 0.5
}

struct GroupChatRowView: View {
    let row: ChatRow
    let now: Date
    var members: [Components.Schemas.CabalMember] = []
    var cabalName = ""
    var proposals: ChatProposalStore?
    let openProfile: (String) -> Void
    let retry: (String) -> Void
    var discard: ((String) -> Void)?
    var openThread: ((String) -> Void)?
    var openProposal: ((String) -> Void)?
    var replyHere: (() -> Void)?
    var requestDelete: ((String) -> Void)?
    var seenLabel: String?
    var openSeen: (() -> Void)?
    var reactions: ReactionsModel?
    var cabalID = ""

    @Environment(ToastCenter.self) private var toasts
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var reporting: ReportTarget?
    @State private var blocking: BlockTarget?
    @State private var actionsFrame: CGRect = .zero
    @State private var actionsOpen = false
    @State private var pickingEmoji = false
    @State private var listingReactions = false
    @State private var afterDismiss: (() -> Void)?
    @State private var badgeHeight: CGFloat = 0

    private var message: ChatMessage { row.message }

    private var blockAction: (() -> Void)? {
        let author = message.author
        guard !row.isMine, !message.deleted, !author.displayName.isEmpty else { return nil }
        return { blocking = BlockTarget(userID: author.id, name: author.displayName, handle: author.handle ?? "") }
    }

    var body: some View {
        VStack(spacing: 0) {
            if let label = row.dayLabel(now: now) {
                ChatDayLabel(label: label)
                    .accessibilityIdentifier("chat-day-\(row.id)")
            }
            if let proposalID = message.proposalId, let proposals {
                ChatProposalCard(
                    message: message, proposalID: proposalID, store: proposals, members: members,
                    cabalName: cabalName, now: now, openProposal: { openProposal?(proposalID) },
                    openThread: openThread.map { open in { open(message.id) } })
            } else {
                messageRow
                    .padding(
                        .top,
                        row.startsDay ? 0 : (row.startsRun ? ChatBubbleMetrics.groupGap : ChatBubbleMetrics.runGap))
            }
        }
        .frame(maxWidth: .infinity)
    }

    private var messageRow: some View {
        VStack(alignment: row.isMine ? .trailing : .leading, spacing: MonacoTheme.Space.xs) {
            threadHeader.padding(.leading, columnInset)
            if Self.showsAuthorName(startsRun: row.startsRun, isMine: row.isMine) {
                authorLine.padding(.leading, columnInset + ChatBubbleMetrics.nameInset)
            }
            HStack(alignment: .bottom, spacing: MonacoTheme.Space.s) {
                if !row.isMine { avatarColumn }
                reactable(bubble)
                    .reportSheet($reporting)
                    .blockConfirmation($blocking)
                    .modifier(
                        ChatFailedMenu(
                            enabled: row.delivery == .failed, retry: { retry(row.id) },
                            discard: discard.map { discard in { discard(row.id) } }))
            }
            statusLine.padding(.leading, columnInset)
            if message.replyCount > 0 { repliesRow.padding(.leading, columnInset) }
        }
        .containerRelativeFrame(.horizontal, alignment: row.isMine ? .trailing : .leading) { length, _ in
            bubbleWidth(in: length)
        }
        .frame(maxWidth: .infinity, alignment: row.isMine ? .trailing : .leading)
    }

    private var columnInset: CGFloat {
        row.isMine ? 0 : ChatBubbleMetrics.avatarSize + MonacoTheme.Space.s
    }

    static func showsAuthorName(startsRun: Bool, isMine: Bool) -> Bool { startsRun && !isMine }

    static func showsFace(endsRun: Bool, isMine: Bool) -> Bool { endsRun && !isMine }

    static func bubbleFill(isMine: Bool) -> Color {
        isMine ? MonacoTheme.chatBubbleMine : MonacoTheme.surfaceSunken
    }

    private func bubbleWidth(in length: CGFloat) -> CGFloat {
        let content = length - MonacoTheme.Space.gutter * 2
        let fraction = row.isMine ? ChatBubbleMetrics.mineWidthFraction : ChatBubbleMetrics.widthFraction
        return max(content * fraction, 0)
    }

    @ViewBuilder private var avatarColumn: some View {
        let author = message.author
        if Self.showsFace(endsRun: row.endsRun, isMine: row.isMine) {
            Button {
                openProfile(author.id)
            } label: {
                MonacoAvatar(
                    photoURL: author.photoUrl, displayName: author.displayName, size: ChatBubbleMetrics.avatarSize,
                    seed: author.id)
            }
            .buttonStyle(.plain)
            .contentShape(Circle().inset(by: ChatBubbleMetrics.avatarHitInset))
            .disabled(author.displayName.isEmpty)
            .accessibilityLabel(
                author.displayName.isEmpty ? "Former member" : "\(author.displayName), open profile"
            )
            .accessibilityIdentifier("chat-avatar-\(row.id)")
        } else {
            Color.clear.frame(width: ChatBubbleMetrics.avatarSize, height: ChatBubbleMetrics.avatarSize)
        }
    }

    private var bubble: some View {
        bubbleText
            .padding(.horizontal, MonacoTheme.Space.sm)
            .padding(.vertical, MonacoTheme.Space.s)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.compact, style: .continuous)
                    .fill(Self.bubbleFill(isMine: row.isMine))
            )
            .opacity(row.delivery == .pending ? ChatBubbleMetrics.pendingOpacity : ChatBubbleMetrics.sentOpacity)
            .accessibilityElement(children: .combine)
            .accessibilityLabel(accessibilityText)
            .accessibilityIdentifier("chat-message-\(row.id)")
    }

    @ViewBuilder private var authorLine: some View {
        let author = message.author
        let named = !author.displayName.isEmpty
        Button {
            openProfile(author.id)
        } label: {
            Text(named ? author.displayName : "Former member")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(1)
                .monacoPrivate()
        }
        .buttonStyle(.plain)
        .disabled(!named)
        .accessibilityHidden(true)
        .accessibilityIdentifier("chat-author-\(row.id)")
    }

    @ViewBuilder private var bubbleText: some View {
        if message.deleted {
            Text(GroupChatCopy.deleted)
                .font(MonacoTheme.Typo.subhead)
                .italic()
                .foregroundStyle(MonacoTheme.secondaryText)
        } else {
            ChatMessageText(
                text: message.body ?? "", members: members,
                color: MonacoTheme.primaryText,
                mentionColor: MonacoTheme.tint, openProfile: openProfile)
        }
    }
}

extension GroupChatRowView {
    @ViewBuilder private var statusLine: some View {
        switch row.delivery {
        case .failed:
            HStack(spacing: MonacoTheme.Space.sm) {
                Text(GroupChatCopy.notSent)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                Button {
                    retry(row.id)
                } label: {
                    Text(GroupChatCopy.retry)
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.brand)
                        .frame(minWidth: MonacoButtonMetrics.hitTarget, minHeight: MonacoButtonMetrics.hitTarget)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("chat-retry-\(row.id)")
            }
        case .pending:
            if row.isMine {
                caption(GroupChatCopy.sending)
                    .accessibilityIdentifier("chat-time-\(row.id)")
            }
        case .sent:
            if row.endsRun { sentLine }
        }
    }

    @ViewBuilder private var sentLine: some View {
        HStack(spacing: 0) {
            Text(timeText)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityHidden(true)
                .accessibilityIdentifier("chat-time-\(row.id)")
            seenFooter
        }
    }

    private var timeText: String {
        let time = row.date.formatted(.dateTime.hour().minute())
        return row.isMine ? GroupChatCopy.statusLine(time: time, status: GroupChatCopy.sent) : time
    }

    private func caption(_ text: String) -> some View {
        Text(text)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .accessibilityHidden(true)
    }

    @ViewBuilder private var seenFooter: some View {
        if let seenLabel, let openSeen {
            Text(" · ")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityHidden(true)
            Button(action: openSeen) {
                Text(seenLabel)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .frame(minHeight: MonacoButtonMetrics.hitTarget)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("chat-seen-label")
        }
    }

    private var rootID: String { message.parentId ?? message.id }

    private var canOpenMenu: Bool {
        (openThread != nil || replyHere != nil) && !message.deleted && row.delivery == .sent
    }

    @ViewBuilder private var threadHeader: some View {
        if message.alsoInChannel, let parentID = message.parentId, let openThread {
            Button {
                openThread(parentID)
            } label: {
                HStack(spacing: MonacoTheme.Space.xs) {
                    Image(systemName: "arrowshape.turn.up.left")
                    Text(ChatThreadCopy.header(parentBody: row.parentBody)).lineLimit(1)
                }
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(minHeight: MonacoButtonMetrics.hitTarget)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("chat-thread-header-\(row.id)")
        }
    }

    @ViewBuilder private var repliesRow: some View {
        let summary = ChatThreadCopy.summary(
            replyCount: message.replyCount, lastReplyAt: message.lastReplyAt, now: now)
        if let openThread {
            Button {
                openThread(message.id)
            } label: {
                HStack(spacing: MonacoTheme.Space.xs) {
                    Text(summary)
                    Image(systemName: "chevron.right").accessibilityHidden(true)
                }
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.brand)
                .frame(minHeight: MonacoButtonMetrics.hitTarget)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("chat-replies-\(row.id)")
        } else {
            Text(summary)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.brand)
                .accessibilityIdentifier("chat-replies-\(row.id)")
        }
    }

    private var accessibilityText: String {
        let who = row.isMine ? "You" : message.author.displayName
        let body = message.deleted ? GroupChatCopy.deleted : (message.body ?? "")
        let time = row.date.formatted(date: .omitted, time: .shortened)
        let state: String
        switch row.delivery {
        case .pending: state = " \(GroupChatCopy.sending)"
        case .failed: state = " \(GroupChatCopy.notSent)"
        case .sent: state = row.isMine && row.endsRun ? " \(GroupChatCopy.sent)" : ""
        }
        return who.isEmpty ? "\(time): \(body).\(state)" : "\(who), \(time): \(body).\(state)"
    }
}

extension GroupChatRowView {
    private var target: ReactionTarget { .chatMessage(cabalID: cabalID, id: message.id) }

    private var liveReactions: [Components.Schemas.Reaction] {
        reactions?.reactions(for: target, fallback: message.reactions) ?? message.reactions
    }

    private var canReact: Bool { reactions != nil && !message.deleted && row.delivery == .sent }

    private var hasBadge: Bool { !message.deleted && !liveReactions.isEmpty }

    private var opensActions: Bool { canReact || canOpenMenu }

    private func reactable(_ content: some View) -> some View {
        content
            .onGeometryChange(
                for: CGRect.self,
                of: { $0.frame(in: .global) },
                action: { actionsFrame = $0 }
            )
            .overlay(alignment: row.isMine ? .topLeading : .topTrailing) {
                if hasBadge { badge }
            }
            .padding(.top, hasBadge ? max(badgeHeight - ChatTapbackMetrics.overlap, ChatTapbackMetrics.topSpace) : 0)
            .onLongPressGesture(minimumDuration: 0.4) {
                if opensActions { presentActions() }
            }
            .accessibilityAction(named: "React") {
                if canReact { presentActions() }
            }
            .fullScreenCover(isPresented: $actionsOpen, onDismiss: runAfterDismiss) {
                actionsOverlay.presentationBackground(.clear)
            }
            .sheet(isPresented: $pickingEmoji) {
                EmojiPickerSheet(onPick: { emoji in react(emoji) })
            }
            .sheet(isPresented: $listingReactions) { reactionList }
    }

    private var badge: some View {
        ChatTapbackBadge(
            messageID: message.id, reactions: liveReactions,
            onTap: {
                guard canReact else { return }
                switch ChatTapbackBadge.tapAction(liveReactions) {
                case .remove(let emoji): react(emoji)
                case .openTray: presentActions()
                }
            },
            onLongPress: { listingReactions = true }
        )
        .onGeometryChange(for: CGFloat.self, of: { $0.size.height }, action: { badgeHeight = $0 })
        .offset(
            x: row.isMine ? -ChatTapbackMetrics.out : ChatTapbackMetrics.out,
            y: ChatTapbackMetrics.overlap - badgeHeight)
    }

    private var reactionList: some View {
        VStack {
            ReactionRow(
                target: target, reactions: liveReactions, opensTrayOnLongPress: false,
                onToggle: { emoji in react(emoji) }
            ) { EmptyView() }
            .padding(MonacoTheme.Space.gutter)
            Spacer(minLength: 0)
        }
        .presentationDetents([.height(160)])
        .accessibilityIdentifier("chat-tapback-list-\(message.id)")
    }

    private func react(_ emoji: String) {
        guard canReact, let reactions else { return }
        let current = liveReactions
        Task { await reactions.toggle(emoji, on: target, current: current) }
    }

    private func presentActions() {
        var transaction = Transaction()
        transaction.disablesAnimations = reduceMotion
        withTransaction(transaction) { actionsOpen = true }
    }

    private func dismissActions() {
        var transaction = Transaction()
        transaction.disablesAnimations = true
        withTransaction(transaction) { actionsOpen = false }
    }

    private func runAfterDismiss() {
        let work = afterDismiss
        afterDismiss = nil
        work?()
    }

    private var actionItems: [ChatMessageAction] {
        guard canOpenMenu else { return [] }
        var items = [
            ChatMessageAction(id: "reply", title: ChatThreadCopy.reply, icon: .replyArrow) {
                afterDismiss = replyHere ?? { openThread?(rootID) }
            },
            ChatMessageAction(id: "copy", title: GroupChatCopy.copy, icon: .copy) {
                afterDismiss = {
                    UIPasteboard.general.string = message.body
                    toasts.show(success: "Copied")
                }
            },
        ]
        if !row.isMine {
            items.append(
                ChatMessageAction(id: "report", title: "Report", icon: .report) {
                    afterDismiss = { reporting = ReportTarget(kind: .message, targetID: row.id) }
                })
        }
        if let block = blockAction {
            items.append(
                ChatMessageAction(id: "block", title: "Block", icon: .hand, destructive: true) {
                    afterDismiss = block
                })
        }
        if row.isMine, let requestDelete {
            items.append(
                ChatMessageAction(id: "delete", title: ChatThreadCopy.delete, icon: .trash, destructive: true) {
                    afterDismiss = { requestDelete(row.id) }
                })
        }
        return items
    }

    private var actionsOverlay: some View {
        ChatMessageActionsOverlay(
            messageID: message.id, target: target, frame: actionsFrame, isMine: row.isMine,
            actions: actionItems,
            onPick: { emoji in afterDismiss = { react(emoji) } },
            onOpenPicker: { afterDismiss = { pickingEmoji = true } },
            onDismiss: { dismissActions() },
            bubble: { bubble }
        )
    }
}

struct ChatDayLabel: View {
    let label: String

    var body: some View {
        Text(label)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .multilineTextAlignment(.center)
            .lineLimit(2)
            .frame(maxWidth: .infinity)
            .padding(.top, MonacoTheme.Space.s)
            .padding(.bottom, MonacoTheme.Space.sm)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(label)
    }
}

struct ChatFailedMenu: ViewModifier {
    let enabled: Bool
    let retry: () -> Void
    let discard: (() -> Void)?

    @ViewBuilder func body(content: Content) -> some View {
        if enabled, let discard {
            content.contextMenu {
                Button(action: retry) {
                    Label(GroupChatCopy.retry, systemImage: "arrow.clockwise")
                }
                Button(role: .destructive, action: discard) {
                    Label(ChatThreadCopy.delete, systemImage: "trash")
                }
            }
        } else {
            content
        }
    }
}

extension View {
    func chatDeleteConfirmation(messageID: Binding<String?>, confirm: @escaping (String) -> Void) -> some View {
        confirmationDialog(
            ChatThreadCopy.deleteTitle,
            isPresented: Binding(
                get: { messageID.wrappedValue != nil }, set: { if !$0 { messageID.wrappedValue = nil } }),
            titleVisibility: .visible,
            presenting: messageID.wrappedValue
        ) { id in
            Button(ChatThreadCopy.delete, role: .destructive) { confirm(id) }
            Button(ChatThreadCopy.cancel, role: .cancel) {}
        } message: { _ in
            Text(ChatThreadCopy.deleteMessage)
        }
    }
}
