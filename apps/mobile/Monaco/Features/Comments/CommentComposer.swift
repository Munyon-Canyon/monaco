import MonacoCore
import SwiftUI

struct CommentComposerBar<Restricted: View>: View {
    @Bindable var model: CommentsModel
    var replyOwner: String?
    var menuItems: [ComposerMenuItem] = []
    var thesis: String?
    var offersPoll = false
    var asksCabal = false
    var onDidStandDown: () -> Void = {}
    @ViewBuilder var restricted: () -> Restricted

    @State private var picksGif = false
    @State private var picksPoll = false

    var body: some View {
        VStack(spacing: 0) {
            if model.replySheet == nil { bar }
        }
        .sheet(item: sheetBinding) { target in
            ReplySheet(
                model: model, target: target, owner: replyOwner, menuItems: menuItems, thesis: thesis,
                offersPoll: offersPoll, asksCabal: asksCabal)
        }
        .gifPicker(isPresented: $picksGif, model: model)
        .pollComposer(isPresented: $picksPoll, model: model, isCabal: asksCabal)
    }

    private var sheetBinding: Binding<ReplyTarget?> {
        Binding(
            get: { model.replySheet },
            set: { if $0 == nil { model.dismissReplySheet() } })
    }

    @ViewBuilder private var bar: some View {
        if model.canCommentFailed {
            MonacoErrorRow(thing: "the comment box", identifier: "comment-composer-error") {
                Task { await model.retryCanComment() }
            }
            .background(MonacoTheme.canvas.ignoresSafeArea(edges: .bottom))
            .overlay(alignment: .top) { MonacoRule() }
        } else if !model.canCommentKnown {
            EmptyView()
        } else if model.canComment {
            DockedComposer(
                model: model, menuItems: menuItems, thesis: thesis, onPoll: offersPoll ? { picksPoll = true } : nil,
                onGif: { picksGif = true }, onDidStandDown: onDidStandDown)
        } else if case .members(let cabalName, let pictureURL) = model.replyRule {
            MembersReplyLine(cabalName: cabalName, pictureURL: pictureURL)
        } else {
            restricted()
        }
    }
}

extension CommentComposerBar where Restricted == EmptyView {
    init(
        model: CommentsModel, replyOwner: String? = nil, menuItems: [ComposerMenuItem] = [], thesis: String? = nil,
        offersPoll: Bool = false, asksCabal: Bool = false, onDidStandDown: @escaping () -> Void = {}
    ) {
        self.init(
            model: model, replyOwner: replyOwner, menuItems: menuItems, thesis: thesis, offersPoll: offersPoll,
            asksCabal: asksCabal, onDidStandDown: onDidStandDown,
            restricted: { EmptyView() })
    }
}

private struct DockedComposer: View {
    @Bindable var model: CommentsModel
    let menuItems: [ComposerMenuItem]
    let thesis: String?
    let onPoll: (() -> Void)?
    let onGif: () -> Void
    let onDidStandDown: () -> Void

    @FocusState private var focused: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            ComposerNotices(model: model)
            ComposerAttachmentSlot(model: model)
            HStack(alignment: .bottom, spacing: MonacoTheme.Space.s) {
                if model.draft.isEmpty {
                    ComposerMenu(items: menuItems, target: .thread, thesis: thesis, model: model, onPoll: onPoll)
                }
                ComposerField(
                    model: model, focused: $focused,
                    onGif: {
                        focused = false
                        onGif()
                    })
                ComposerSendButton(model: model) {
                    focused = false
                    onDidStandDown()
                }
            }
            ComposerTooLong(draft: CommentDraft(text: model.draft))
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .background(MonacoTheme.canvas.ignoresSafeArea(edges: .bottom))
        .overlay(alignment: .top) {
            MonacoRule()
        }
        .onChange(of: model.isDockedFocused) { _, wanted in focused = wanted }
        .onChange(of: focused) { _, isFocused in model.isDockedFocused = isFocused }
        .onAppear { focused = model.isDockedFocused }
    }
}

struct ComposerNotices: View {
    let model: CommentsModel

    var body: some View {
        if model.isPosting {
            MonacoNotice(.info, message: CommentsCopy.sending)
                .accessibilityIdentifier("comment-composer-sending")
        } else if model.sendFailed {
            MonacoNotice(.error, message: CommentsCopy.sendFailed)
                .accessibilityIdentifier("comment-composer-failed")
        }
    }
}

struct ComposerTooLong: View {
    let draft: CommentDraft

    var body: some View {
        if case .tooLong = draft {
            Text(CommentsCopy.tooLong)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.loss)
                .accessibilityIdentifier("comment-composer-too-long")
        }
    }
}

struct ComposerField: View {
    let model: CommentsModel
    var focused: FocusState<Bool>.Binding
    var onGif: (() -> Void)?

    private var text: Binding<String> {
        Binding(get: { model.draft }, set: { model.edit($0) })
    }

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: CommentRowMetrics.replyTarget / 2, style: .continuous)
        HStack(alignment: .bottom, spacing: 0) {
            TextField(
                "",
                text: text,
                prompt: Text(CommentsCopy.placeholder).foregroundStyle(MonacoTheme.secondaryText),
                axis: .vertical
            )
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.primaryText)
            .tint(MonacoTheme.primaryText)
            .lineLimit(1...5)
            .focused(focused)
            .padding(.leading, MonacoTheme.Space.m)
            .padding(.vertical, MonacoTheme.Space.sm)
            .frame(minHeight: CommentRowMetrics.replyTarget)
            .contentShape(Rectangle())
            .accessibilityIdentifier("comment-composer-field")
            .monacoPrivate()
            if let onGif { ComposerGifButton(action: onGif) }
        }
        .padding(.trailing, onGif == nil ? MonacoTheme.Space.m : MonacoTheme.Space.sm)
        .frame(minHeight: CommentRowMetrics.replyTarget)
        .background(shape.fill(MonacoTheme.surfaceSunken))
        .overlay(shape.strokeBorder(MonacoTheme.border, lineWidth: 1))
    }
}

private struct ComposerGifButton: View {
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Text(CommentsCopy.gifTitle)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.horizontal, MonacoTheme.Space.s)
                .padding(.vertical, MonacoTheme.Space.xs)
                .overlay(
                    RoundedRectangle(cornerRadius: MonacoTheme.Radius.pill, style: .continuous)
                        .strokeBorder(MonacoTheme.secondaryText, lineWidth: 1)
                )
                .frame(minWidth: CommentRowMetrics.replyTarget, minHeight: CommentRowMetrics.replyTarget)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .frame(minWidth: CommentRowMetrics.replyTarget, minHeight: CommentRowMetrics.replyTarget)
        .contentShape(Rectangle())
        .accessibilityLabel(CommentsCopy.gifAccessibility)
        .accessibilityIdentifier("comment-composer-gif")
    }
}

struct ComposerSendButton: View {
    let model: CommentsModel
    var onSent: () -> Void = {}

    private var draft: CommentDraft {
        CommentDraft(text: model.draft)
    }

    private var canPost: Bool {
        (draft.body != nil || model.attachmentDraft != nil) && !model.isPosting
    }

    var body: some View {
        Button {
            Task {
                guard await model.post(draft), model.draft.isEmpty else { return }
                onSent()
            }
        } label: {
            ComposerSendDisc(isLive: canPost || model.isPosting, isSending: model.isPosting)
        }
        .buttonStyle(.plain)
        .frame(minWidth: CommentRowMetrics.replyTarget, minHeight: CommentRowMetrics.replyTarget)
        .contentShape(Rectangle())
        .disabled(!canPost)
        .accessibilityLabel(CommentsCopy.postAccessibility)
        .accessibilityIdentifier("comment-composer-send")
    }
}

struct ComposerSendDisc: View {
    let isLive: Bool
    let isSending: Bool

    var body: some View {
        ZStack {
            Circle()
                .fill(MonacoTheme.primaryButtonFill)
            if isSending {
                ProgressView()
                    .tint(MonacoTheme.onBrand)
            } else {
                Image(monaco: .up)
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.primaryButtonLabel)
            }
        }
        .frame(width: CommentRowMetrics.replyTarget, height: CommentRowMetrics.replyTarget)
        .opacity(isLive ? 1 : ComposerSendDisc.idleOpacity)
        .animation(.easeOut(duration: 0.15), value: isLive)
    }

    static let idleOpacity = MonacoButtonMetrics.disabledFillOpacity
}
