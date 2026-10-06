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
    let retry: () -> Void

    var body: some View {
        VStack(spacing: 12) {
            Text(GroupChatCopy.loadFailure)
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
    private let bubbles: [(mine: Bool, width: CGFloat)] = [(false, 188), (true, 152), (false, 216)]

    var body: some View {
        VStack(spacing: 6) {
            Spacer(minLength: 0)
            ForEach(Array(bubbles.enumerated()), id: \.offset) { _, bubble in
                HStack(spacing: 0) {
                    if bubble.mine { Spacer(minLength: 56) }
                    SkeletonBlock(width: bubble.width, height: 38, radius: MonacoTheme.Radius.bubble)
                    if !bubble.mine { Spacer(minLength: 56) }
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.bottom, MonacoTheme.Space.sm)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading messages")
        .accessibilityIdentifier("chat-loading")
    }
}

struct ChatComposerBar: View {
    var focus: FocusState<Bool>.Binding
    let send: (String) -> Void

    @State private var draft = ""

    private var trimmedCount: Int {
        draft.trimmingCharacters(in: .whitespacesAndNewlines).unicodeScalars.count
    }

    private var canSend: Bool {
        (try? GroupChatDraft.validate(draft).get()) != nil
    }

    var body: some View {
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
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.s)
        .background(MonacoTheme.background)
        .overlay(alignment: .top) { MonacoRule() }
    }

    private var field: some View {
        TextField(
            GroupChatCopy.composerPlaceholder,
            text: $draft,
            prompt: Text(GroupChatCopy.composerPlaceholder).foregroundStyle(MonacoTheme.disabledLabel),
            axis: .vertical
        )
        .font(MonacoTheme.Typo.body)
        .foregroundStyle(MonacoTheme.ink)
        .tint(MonacoTheme.ink)
        .lineLimit(1...5)
        .focused(focus)
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, 11)
        .frame(minHeight: 44)
        .background(MonacoTheme.surfaceSunken, in: RoundedRectangle(cornerRadius: 22, style: .continuous))
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

    private func submit() {
        guard case .success(let body) = GroupChatDraft.validate(draft) else { return }
        draft = ""
        send(body)
    }
}
