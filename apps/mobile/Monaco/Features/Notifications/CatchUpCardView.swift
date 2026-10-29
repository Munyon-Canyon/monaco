import MonacoAPI
import MonacoAnalytics
import MonacoCore
import MonacoNotify
import SwiftUI

struct CatchUpDeckStart: Identifiable, Equatable {
    let key: String?
    var scrolledToEnd = false
    var id: String { key ?? "first" }
}

struct CatchUpCardView: View {
    let model: CatchUpModel
    let now: Date
    let rowActions: CatchUpRowActions
    let seeAll: () -> Void

    @State private var deck: CatchUpDeckModel
    @State private var reactions: ReactionsModel
    @State private var replying: CommentsModel?
    @State private var drag: CGFloat = 0
    @State private var leaving: Edge = .trailing

    @Environment(\.dismiss) private var dismiss
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts

    private let scrollAnchor: UnitPoint?
    private static let threshold: CGFloat = 96
    private static let dock: CGFloat = 56

    init(
        model: CatchUpModel, screen: CatchUpScreen, start: CatchUpDeckStart, now: Date, rowActions: CatchUpRowActions,
        reactions: ReactionsModel, seeAll: @escaping () -> Void
    ) {
        self.model = model
        self.now = now
        self.rowActions = rowActions
        self.seeAll = seeAll
        _deck = State(initialValue: CatchUpDeckModel(catchUp: model, screen: screen, startingAt: start.key))
        scrollAnchor = start.scrolledToEnd ? .bottom : nil
        _reactions = State(initialValue: reactions)
    }

    var body: some View {
        VStack(spacing: 0) {
            bar
            ScrollView {
                VStack(spacing: MonacoTheme.Space.l) {
                    card
                    CatchUpCardUpNext(items: deck.deck.upNext, now: now, onSeeAll: close(then: seeAll))
                }
                .padding(.top, MonacoTheme.Space.s)
            }
            .scrollIndicators(.hidden)
            .defaultScrollAnchor(scrollAnchor)
        }
        .safeAreaInset(edge: .bottom, spacing: 0) { dock }
        .monacoCanvas()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("catch-up-deck")
        .onChange(of: model.state) { deck.sync() }
        .onChange(of: deck.deck.isFinished) { _, finished in if finished { dismiss() } }
        .sheet(item: replySheet) { target in
            if let replying { ReplySheet(model: replying, target: target) }
        }
    }

    private var bar: some View {
        ZStack {
            Text(deck.deck.position)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("catch-up-deck-position")
            HStack {
                MonacoGlassButton(systemImage: "xmark", accessibilityLabel: CatchUpDeckCopy.close) { dismiss() }
                    .accessibilityIdentifier("catch-up-deck-close")
                Spacer()
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .background(MonacoTheme.canvas)
    }

    @ViewBuilder private var card: some View {
        if let item = deck.deck.current {
            CatchUpCardContent(
                item: item, now: now, reactions: reactions,
                actions: CatchUpCardActions(
                    reply: reply, open: close(then: rowActions.open), vote: close(then: rowActions.vote),
                    join: close(then: rowActions.join))
            )
            .padding(MonacoTheme.Space.cardPadding)
            .background(MonacoTheme.surface, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
            .background(alignment: .bottom) { edges }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.s)
            .id(item.key)
            .offset(x: drag)
            .gesture(swipe, including: .all)
            .transition(transition)
            .animation(reduceMotion ? .easeInOut(duration: 0.2) : .easeOut(duration: 0.25), value: item.key)
            .accessibilityAction(named: CatchUpDeckCopy.keep) { act(.keep) }
            .accessibilityAction(named: CatchUpDeckCopy.clear) { act(.clear) }
        }
    }

    private var edges: some View {
        ZStack(alignment: .bottom) {
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card)
                .fill(MonacoTheme.surface.opacity(0.6))
                .padding(.horizontal, MonacoTheme.Space.xl)
                .offset(y: MonacoTheme.Space.m)
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card)
                .fill(MonacoTheme.surface.opacity(0.8))
                .padding(.horizontal, MonacoTheme.Space.l)
                .offset(y: MonacoTheme.Space.s)
        }
        .accessibilityHidden(true)
    }

    private var transition: AnyTransition {
        if reduceMotion { return .opacity }
        return .asymmetric(insertion: .opacity, removal: .move(edge: leaving).combined(with: .opacity))
    }

    private var swipe: some Gesture {
        DragGesture(minimumDistance: 24)
            .onChanged { value in
                guard abs(value.translation.width) > abs(value.translation.height) else { return }
                drag = reduceMotion ? 0 : value.translation.width
            }
            .onEnded { value in
                let width = value.translation.width
                guard abs(width) > Self.threshold, abs(width) > abs(value.translation.height) else {
                    withAnimation(.easeOut(duration: 0.15)) { drag = 0 }
                    return
                }
                drag = 0
                act(width > 0 ? .clear : .keep)
            }
    }

    private var dockLayout: AnyLayout {
        dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.sm))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
    }

    private var dock: some View {
        dockLayout {
            Button {
                act(.keep)
            } label: {
                Label(CatchUpDeckCopy.keep, systemImage: "bookmark").frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoSecondary)
            .accessibilityIdentifier("catch-up-deck-keep")
            Button {
                act(.clear)
            } label: {
                Label(CatchUpDeckCopy.clear, systemImage: "checkmark").frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoPrimary)
            .accessibilityIdentifier("catch-up-deck-clear")
        }
        .frame(minHeight: Self.dock)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .background(MonacoTheme.canvas)
    }

    private enum Verb {
        case keep, clear
    }

    private func act(_ verb: Verb) {
        guard let item = deck.deck.current else { return }
        Haptics.tap()
        leaving = verb == .clear ? .trailing : .leading
        let deck = deck
        let toasts = toasts
        if verb == .clear {
            AppAnalytics.current.step(.catchUp(.cleared), properties: ["kind": .string(item.kind.rawValue)])
        }
        Task {
            let done = verb == .clear ? await deck.clear() : await deck.keep()
            if !done { toasts.current = MonacoToast(message: CatchUpCopy.clearFailedToast) }
        }
    }

    private func close<Item>(then action: @escaping (Item) -> Void) -> (Item) -> Void {
        { item in
            dismiss()
            action(item)
        }
    }

    private func close(then action: @escaping () -> Void) -> () -> Void {
        {
            dismiss()
            action()
        }
    }

    private func reply(_ item: Components.Schemas.CatchUpItem) {
        guard let comment = item.comment else { return }
        let source: any CommentsSource =
            comment.proposalId.map { ProposalCommentsSource(proposalID: $0, api: environment.api) }
            ?? FeedCommentsSource(itemID: comment.feedItemId, api: environment.api)
        let comments = CommentsModel(
            source: source, hints: environment.hints, clock: ContinuousClock(), reactions: reactions)
        comments.presentReplySheet(.comment(CatchUpCardReply.row(comment)))
        replying = comments
    }

    private var replySheet: Binding<ReplyTarget?> {
        Binding(
            get: { replying?.replySheet },
            set: { if $0 == nil { replying?.dismissReplySheet() } })
    }
}

enum CatchUpCardReply {
    static func row(_ comment: Components.Schemas.CatchUpComment) -> CommentThreadRow {
        let author = Components.Schemas.CommentAuthor(
            kind: .user, id: comment.author.id, handle: nil, displayName: comment.author.name,
            photoUrl: comment.author.photoUrl, return1mBps: nil, returnPrivate: false)
        let posted = Components.Schemas.Comment(
            id: comment.commentId, parentCommentId: comment.parentCommentId, rootCommentId: comment.rootCommentId,
            author: author, replyToHandle: nil, body: comment.body, bodyDisplay: comment.body, isMine: false,
            reactions: comment.reactions, reactionCount: comment.reactionCount, createdAt: comment.createdAt)
        return CommentThreadRow(comment: posted, isReply: false)
    }
}
