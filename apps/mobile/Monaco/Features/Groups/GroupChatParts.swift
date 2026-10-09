import MonacoAPI
import MonacoCore
import SwiftUI

struct GroupChatEmptyView: View {
    let cabalID: String
    let title: String
    let pictureUrl: String?

    var body: some View {
        VStack(spacing: 12) {
            CabalMark(groupId: cabalID, name: title, size: 56, pictureUrl: pictureUrl)
            Text(title)
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(.center)
            Text(GroupChatCopy.emptyState)
                .font(MonacoTheme.Typo.callout)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(24)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("chat-empty")
    }
}

struct GroupChatLoadFailureView: View {
    var message = GroupChatCopy.loadFailure
    let retry: () -> Void

    var body: some View {
        VStack(spacing: 12) {
            Text(message)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("chat-error")
            Button("Try again", action: retry)
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("chat-retry")
        }
        .padding(24)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .contain)
    }
}

struct GroupChatClosedNotice: View {
    var body: some View {
        Text(GroupChatCopy.closed)
            .font(MonacoTheme.Typo.callout)
            .foregroundStyle(MonacoTheme.muted)
            .multilineTextAlignment(.center)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.m)
            .background(MonacoTheme.background)
            .overlay(alignment: .top) { MonacoRule() }
            .accessibilityIdentifier("chat-closed")
    }
}

struct ChatSkeleton: View {
    var bottomAligned = true
    private let bubbles: [(mine: Bool, width: CGFloat)] = [(false, 188), (true, 152), (false, 216)]

    var body: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            if bottomAligned { Spacer(minLength: 0) }
            ForEach(Array(bubbles.enumerated()), id: \.offset) { _, bubble in
                HStack(spacing: 0) {
                    if bubble.mine { Spacer(minLength: 56) }
                    SkeletonBlock(width: bubble.width, height: 38, radius: MonacoTheme.Radius.bubble)
                    if !bubble.mine { Spacer(minLength: 56) }
                }
            }
            if !bottomAligned { Spacer(minLength: 0) }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.sm)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading messages")
        .accessibilityIdentifier("chat-loading")
    }
}

struct ChatLoadEarlierButton: View {
    let isLoading: Bool
    let identifier: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            if isLoading {
                ProgressView().tint(MonacoTheme.accent)
            } else {
                Text(GroupChatCopy.loadEarlier).font(MonacoTheme.Typo.calloutStrong)
            }
        }
        .buttonStyle(.borderless)
        .foregroundStyle(MonacoTheme.brand)
        .frame(maxWidth: .infinity, minHeight: 44)
        .contentShape(Rectangle())
        .disabled(isLoading)
        .accessibilityIdentifier(identifier)
    }
}

final class ChatScrollBox {
    var tracker = ChatScrollTracker()
}

struct ChatList<Header: View, Content: View>: View {
    let rows: [ChatRow]
    let hasOlder: Bool
    let isLoadingOlder: Bool
    let loadEarlierID: String
    let listID: String
    let loadOlder: () -> Void
    let refresh: () async -> Void
    @ViewBuilder let header: Header
    @ViewBuilder let content: Content

    @State private var scroll = ChatScrollBox()
    @State private var unreadCount = 0
    @State private var scrollToBottomRequests = 0
    @State private var rowToKeepInView: String?
    @State private var topRowBeforeLoadingOlder: String?

    private static var bottomAnchor: String { "chat-bottom" }

    private var tracker: ChatScrollTracker {
        get { scroll.tracker }
        nonmutating set {
            scroll.tracker = newValue
            if unreadCount != newValue.unreadCount { unreadCount = newValue.unreadCount }
        }
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    header
                    if hasOlder {
                        ChatLoadEarlierButton(isLoading: isLoadingOlder, identifier: loadEarlierID) {
                            topRowBeforeLoadingOlder = rows.first?.id
                            tracker.historyRequested()
                            loadOlder()
                        }
                    }
                    content
                    Color.clear.frame(height: 1).id(Self.bottomAnchor)
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.sm)
            }
            .defaultScrollAnchor(.bottom, for: .initialOffset)
            .defaultScrollAnchor(.bottom, for: .sizeChanges)
            .scrollDismissesKeyboard(.interactively)
            .refreshable { await refresh() }
            .onScrollGeometryChange(for: ChatScrollTracker.Position.self, of: Self.position) { _, updated in
                tracker.positionChanged(updated)
            }
            .onScrollGeometryChange(for: CGFloat.self, of: { $0.contentSize.height }) { old, new in
                guard new > old, tracker.isFollowingThread else { return }
                proxy.scrollTo(Self.bottomAnchor, anchor: .bottom)
            }
            .onScrollPhaseChange { _, phase in track(phase) }
            .onChange(of: rows) { old, new in rowsChanged(from: old, to: new) }
            .onChange(of: isLoadingOlder) { _, loading in
                if !loading, rows.first?.id == topRowBeforeLoadingOlder { topRowBeforeLoadingOlder = nil }
            }
            .onChange(of: scrollToBottomRequests) { _, _ in
                withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo(Self.bottomAnchor, anchor: .bottom) }
            }
            .onChange(of: rowToKeepInView) { _, rowID in
                guard let rowID else { return }
                proxy.scrollTo(rowID, anchor: .top)
                rowToKeepInView = nil
            }
            .accessibilityIdentifier(listID)
            .overlay(alignment: .bottom) {
                ZStack(alignment: .bottom) {
                    if unreadCount > 0 { newMessagesPill }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottom)
                .animation(.snappy, value: unreadCount)
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

    private var newMessagesPill: some View {
        Button {
            tracker.followRequested()
            scrollToBottomRequests += 1
        } label: {
            HStack(spacing: MonacoTheme.Space.s) {
                Image(systemName: "arrow.down").font(.caption.weight(.bold))
                Text(GroupChatCopy.newMessagesPill(count: unreadCount))
                    .font(MonacoTheme.Typo.captionStrong)
            }
            .foregroundStyle(MonacoTheme.primaryButtonLabel)
            .padding(.horizontal, MonacoTheme.Space.m)
            .padding(.vertical, MonacoTheme.Space.sm)
            .background(Capsule().fill(MonacoTheme.primaryButtonFill))
            .frame(minHeight: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.bottom, MonacoTheme.Space.sm)
        .transition(.move(edge: .bottom).combined(with: .opacity))
        .accessibilityIdentifier("chat-new-messages")
    }
}

struct ChatComposerBar: View {
    var focus: FocusState<Bool>.Binding
    var placeholder = GroupChatCopy.composerPlaceholder
    var members: [Components.Schemas.CabalMember] = []
    var viewerID = ""
    let send: (String) -> Void

    @State private var draft = ""
    @State private var selection: TextSelection?

    private var mentionMatches: [Components.Schemas.CabalMember] {
        guard let query = MentionQuery.active(in: draft, cursor: cursorOffset) else { return [] }
        return MentionQuery.matches(query, members: members, viewerID: viewerID)
    }

    private var cursorOffset: Int {
        guard let selection, case .selection(let range) = selection.indices else { return draft.count }
        guard range.upperBound <= draft.endIndex else { return draft.count }
        return draft.distance(from: draft.startIndex, to: range.upperBound)
    }

    private var trimmedCount: Int {
        draft.trimmingCharacters(in: .whitespacesAndNewlines).unicodeScalars.count
    }

    private var canSend: Bool {
        (try? GroupChatDraft.validate(draft).get()) != nil
    }

    var body: some View {
        VStack(spacing: 0) {
            if !mentionMatches.isEmpty { ChatMentionPicker(matches: mentionMatches, pick: insert) }
            composer
        }
    }

    private var composer: some View {
        VStack(alignment: .trailing, spacing: 4) {
            HStack(alignment: .bottom, spacing: MonacoTheme.Space.s) {
                field
                sendButton
            }
            if trimmedCount > GroupChatDraft.maxCharacters - 200 {
                Text("\(trimmedCount)/\(GroupChatDraft.maxCharacters)")
                    .font(MonacoTheme.Typo.stamp)
                    .foregroundStyle(
                        trimmedCount > GroupChatDraft.maxCharacters
                            ? MonacoTheme.destructive : MonacoTheme.secondaryText
                    )
                    .accessibilityIdentifier("chat-char-count")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .background(MonacoTheme.background)
        .overlay(alignment: .top) { MonacoRule() }
    }

    private var field: some View {
        TextField(
            placeholder,
            text: $draft,
            selection: $selection,
            prompt: Text(placeholder).foregroundStyle(MonacoTheme.disabledLabel),
            axis: .vertical
        )
        .font(MonacoTheme.Typo.body)
        .foregroundStyle(MonacoTheme.ink)
        .tint(MonacoTheme.ink)
        .lineLimit(1...5)
        .focused(focus)
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: 44)
        .background(
            MonacoTheme.surfaceSunken,
            in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.field, style: .continuous)
        )
        .accessibilityIdentifier("chat-composer")
    }

    private var sendButton: some View {
        Button {
            Haptics.tap()
            submit()
        } label: {
            ComposerSendDisc(isLive: canSend, isSending: false)
        }
        .buttonStyle(.plain)
        .disabled(!canSend)
        .accessibilityLabel("Send message")
        .accessibilityIdentifier("chat-send")
    }

    private func insert(_ member: Components.Schemas.CabalMember) {
        guard let handle = member.handle else { return }
        let result = MentionInsertion.insert(handle: handle, into: draft, cursor: cursorOffset)
        draft = result.text
        selection = TextSelection(insertionPoint: MentionInsertion.caret(in: draft, offset: result.cursor))
    }

    private func submit() {
        guard case .success(let body) = GroupChatDraft.validate(draft) else { return }
        selection = nil
        draft = ""
        send(body)
    }
}
