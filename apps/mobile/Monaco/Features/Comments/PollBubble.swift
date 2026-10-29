import MonacoAPI
import MonacoCore
import SwiftUI

enum PollBubbleMetrics {
    static let rowHeight: CGFloat = 44
    static let radius = MonacoTheme.Radius.card
    static let maxWidth: CGFloat = 320
}

private struct PollVotingKey: EnvironmentKey {
    static let defaultValue: (any PollVoting)? = nil
}

extension EnvironmentValues {
    var pollVoting: (any PollVoting)? {
        get { self[PollVotingKey.self] }
        set { self[PollVotingKey.self] = newValue }
    }
}

struct PollBubble: View {
    let commentID: String
    let poll: Components.Schemas.PollAttachment

    @Environment(AppEnvironment.self) private var environment: AppEnvironment?
    @Environment(\.pollVoting) private var voting
    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        if let voting = voting ?? environment.map({ LivePollVoting(api: $0.api) }) {
            PollBubbleContent(commentID: commentID, poll: poll, voting: voting, toasts: toasts)
        }
    }
}

private struct PollBubbleContent: View {
    let commentID: String
    let poll: Components.Schemas.PollAttachment
    let toasts: ToastCenter

    @State private var model: CommentPollModel

    init(commentID: String, poll: Components.Schemas.PollAttachment, voting: any PollVoting, toasts: ToastCenter) {
        self.commentID = commentID
        self.poll = poll
        self.toasts = toasts
        _model = State(
            initialValue: CommentPollModel(
                commentID: commentID, poll: poll, now: Date(), voting: voting, clock: ContinuousClock()))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text(model.poll.question)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
            ForEach(model.poll.options, id: \.position) { option in
                optionRow(option)
            }
            Text(model.footer)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("poll-footer-\(commentID)")
        }
        .frame(maxWidth: PollBubbleMetrics.maxWidth, alignment: .leading)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("poll-bubble-\(commentID)")
        .task { await model.run() }
        .onChange(of: poll) { _, next in model.reconcile(next, now: Date()) }
        .onChange(of: model.noticeTick) { _, _ in
            if let failure = model.failure { toasts.show(failure) }
        }
    }

    private func optionRow(_ option: Components.Schemas.PollOption) -> some View {
        let shape = RoundedRectangle(cornerRadius: PollBubbleMetrics.radius, style: .continuous)
        let mine = model.myPosition == option.position
        return Button {
            Task { await model.tap(option.position) }
        } label: {
            ZStack(alignment: .leading) {
                GeometryReader { proxy in
                    Rectangle()
                        .fill(MonacoTheme.brand.opacity(mine ? 0.28 : 0.14))
                        .frame(width: proxy.size.width * CGFloat(model.shareBps(option)) / 10_000)
                }
                HStack(spacing: MonacoTheme.Space.s) {
                    if mine {
                        Image(monaco: .pollCheck).foregroundStyle(MonacoTheme.brand)
                    }
                    Text(option.label)
                        .font(MonacoTheme.Typo.body)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .lineLimit(1)
                    Spacer(minLength: 0)
                    Text("\(model.percent(option))% · \(option.count)")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .padding(.horizontal, MonacoTheme.Space.sm)
            }
            .frame(height: PollBubbleMetrics.rowHeight)
            .background(MonacoTheme.surfaceSunken)
            .clipShape(shape)
            .contentShape(shape)
        }
        .buttonStyle(.plain)
        .disabled(model.isClosed)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            "\(option.label), \(model.percent(option)) percent, \(CommentsCopy.pollVotes(option.count))"
        )
        .accessibilityValue(mine ? "Your vote" : "")
        .accessibilityAddTraits(mine ? .isSelected : [])
        .accessibilityIdentifier("poll-option-\(commentID)-\(option.position)")
    }
}
