import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatQuietMetrics {
    static let sceneHeight: CGFloat = 210
    static let sceneMinimumHeight: CGFloat = 96
    static let sceneShare: CGFloat = 0.36
    static let titleTop: CGFloat = 16
    static let lineTop: CGFloat = 4
    static let headerBottom: CGFloat = 12
    static let actionsTop: CGFloat = 16
    static let rowMinHeight: CGFloat = 44
    static let circle: CGFloat = 32
    static let rowGap: CGFloat = 12
    static let fadeDuration: Double = 0.2

    static func sceneHeight(container: CGFloat) -> CGFloat {
        max(sceneMinimumHeight, min(sceneHeight, container * sceneShare))
    }

    static var ruleInset: CGFloat { MonacoTheme.Space.gutter + circle + rowGap }
}

struct ChatQuietFacts: Equatable {
    var pot: String?
    var figures: CabalPositionRowFigures?

    init(pot: String? = nil, figures: CabalPositionRowFigures? = nil) {
        self.pot = pot
        self.figures = figures
    }

    init(_ payload: Components.Schemas.CabalPot) {
        pot = Self.decimal(micros: payload.potValueMicros)
        guard let me = payload.me else { return }
        let ratio: String? =
            me.netContributedMicros > 0
            ? Self.decimal(micros: me.pnlMicros * 1_000_000 / me.netContributedMicros) : nil
        figures = CabalPositionRowFigures(
            equityUsd: Self.decimal(micros: me.valueMicros), dollarPnl: Self.decimal(micros: me.pnlMicros),
            percentReturn: ratio)
    }

    static func decimal(micros: Int64) -> String {
        let whole = abs(micros) / 1_000_000
        let digits = String(abs(micros) % 1_000_000)
        let fraction = String(repeating: "0", count: 6 - digits.count) + digits
        return "\(micros < 0 ? "-" : "")\(whole).\(fraction)"
    }

    static func load(api: APIClient, cabalID: String) async -> ChatQuietFacts? {
        let payload = try? await api.read { client in
            try await client.getCabalPot(path: .init(id: cabalID)).ok.body.json
        }
        return payload.map(ChatQuietFacts.init)
    }

    func line(memberCount: Int) -> String {
        ChatQuietState.line(memberCount: memberCount, pot: pot, figures: figures)
    }
}

struct ChatQuietContent {
    let kind: ChatQuietState.Kind
    let cabalID: String
    let cabalName: String
    let pictureURL: URL?
    let line: String
    let actions: [ChatQuietState.Action]
    let search: StockSearch?
    let focusComposer: () -> Void
    let openPoll: () -> Void
    let sendStock: (ChatAttachmentDraft) -> Void

    var showsScene: Bool { kind == .fresh }
    var showsActions: Bool { kind != .none }
}

struct ChatQuietHeader: View {
    let content: ChatQuietContent

    var body: some View {
        VStack(spacing: 0) {
            CabalCover(
                cabalId: content.cabalID, tint: .forGroupId(content.cabalID), pictureURL: content.pictureURL,
                style: .scene
            )
            .containerRelativeFrame(.vertical) { length, _ in ChatQuietMetrics.sceneHeight(container: length) }
            .accessibilityHidden(true)
            Text(ChatQuietState.title)
                .font(Font.title3.weight(.semibold))
                .foregroundStyle(MonacoTheme.primaryText)
                .multilineTextAlignment(.center)
                .padding(.top, ChatQuietMetrics.titleTop)
                .accessibilityAddTraits(.isHeader)
                .accessibilityIdentifier("chat-quiet-title")
            Text(Self.styled(content.line))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .padding(.top, ChatQuietMetrics.lineTop)
                .monacoPrivateFigure()
                .accessibilityIdentifier("chat-quiet-line")
        }
        .frame(maxWidth: .infinity)
        .padding(.bottom, ChatQuietMetrics.headerBottom)
        .contentShape(Rectangle())
        .onTapGesture { content.focusComposer() }
        .accessibilityIdentifier("chat-quiet-scene")
    }

    static func styled(_ line: String) -> AttributedString {
        var text = AttributedString(line)
        guard let range = changeRange(in: line), let color = changeColor(of: line[range]),
            let found = text.range(of: String(line[range]), options: .backwards)
        else { return text }
        text[found].foregroundColor = color
        return text
    }

    static func changeRange(in line: String) -> Range<String.Index>? {
        guard let space = line.lastIndex(of: " ") else { return nil }
        let token = line.index(after: space)
        return token < line.endIndex ? token..<line.endIndex : nil
    }

    static func changeColor(of token: Substring) -> Color? {
        switch token.first {
        case "\u{2212}", "-": MonacoTheme.loss
        case "+": MonacoTheme.gain
        default: nil
        }
    }
}

struct ChatQuietActions: View {
    let content: ChatQuietContent

    @State private var showsPropose = false
    @State private var showsStock = false

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
        VStack(spacing: 0) {
            ForEach(Array(content.actions.enumerated()), id: \.element.id) { index, action in
                if index > 0 { MonacoRule().padding(.leading, ChatQuietMetrics.ruleInset) }
                ChatQuietRow(action: action) { open(action.id) }
            }
        }
        .background(shape.fill(MonacoTheme.surface))
        .clipShape(shape)
        .padding(.top, ChatQuietMetrics.actionsTop)
        .proposeChooser(isPresented: $showsPropose, cabalID: content.cabalID)
        .sheet(isPresented: $showsStock) {
            if let search = content.search {
                StockShareSheet(search: search) { pick in
                    guard let draft = ChatAttachmentDraft(pick.input, proposalID: nil) else { return }
                    content.sendStock(draft)
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-quiet-actions")
    }

    private func open(_ id: ChatQuietState.ActionID) {
        switch id {
        case .propose: showsPropose = true
        case .poll: content.openPoll()
        case .shareStock: showsStock = true
        }
    }
}

struct ChatQuietRow: View {
    let action: ChatQuietState.Action
    let open: () -> Void

    @ScaledMetric(relativeTo: .headline) private var circle = ChatQuietMetrics.circle

    var body: some View {
        Button(action: open) {
            HStack(spacing: ChatQuietMetrics.rowGap) {
                mark
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(action.title)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(action.line)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .multilineTextAlignment(.leading)
                Spacer(minLength: 0)
                Image(systemName: "chevron.right")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.tertiaryText)
                    .accessibilityHidden(true)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.sm)
            .frame(maxWidth: .infinity, minHeight: ChatQuietMetrics.rowMinHeight, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spoken(action))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier(Self.identifier(action.id))
    }

    private var mark: some View {
        Image(systemName: Self.symbol(action.id))
            .font(Font.footnote.weight(.semibold))
            .foregroundStyle(action.id == .propose ? MonacoTheme.onAction : MonacoTheme.primaryText)
            .frame(width: circle, height: circle)
            .background(Circle().fill(action.id == .propose ? MonacoTheme.action : MonacoTheme.surfaceSunken))
            .accessibilityHidden(true)
    }

    static func spoken(_ action: ChatQuietState.Action) -> String { "\(action.title). \(action.line)" }

    static func symbol(_ id: ChatQuietState.ActionID) -> String {
        switch id {
        case .propose: MonacoIcon.propose.symbol
        case .poll: "chart.bar"
        case .shareStock: MonacoIcon.markets.symbol
        }
    }

    static func identifier(_ id: ChatQuietState.ActionID) -> String {
        switch id {
        case .propose: "chat-quiet-propose"
        case .poll: "chat-quiet-poll"
        case .shareStock: "chat-quiet-stock"
        }
    }
}

struct ChatQuietGroup<Content: View>: View {
    let visible: Bool
    @ViewBuilder let content: Content

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        Group {
            if visible { content.transition(.opacity) }
        }
        .animation(reduceMotion ? nil : .easeOut(duration: ChatQuietMetrics.fadeDuration), value: visible)
    }
}
