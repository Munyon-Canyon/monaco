#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

final class CommentComposerSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-composerHarness") else { return nil }
        let raw = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "docked"
        let acts = arguments.contains(SampleHarnessRegistry.actArgument)
        let scenario = CommentComposerScenario(rawValue: raw) ?? .docked
        return AnyView(CommentComposerHarnessScreen(scenario: scenario, acts: acts))
    }
}

enum CommentComposerScenario: String {
    case docked
    case sheetComment
    case sheetPost
    case sending
    case failed
    case menu
    case draft
    case menuSale
    case thesisSheet
    case thesisBubble

    var previewMode: FeedItemDetailModel.PreviewMode {
        switch self {
        case .sending: .sending
        case .failed: .failed
        case .docked, .sheetComment, .sheetPost, .menu, .draft, .menuSale, .thesisSheet, .thesisBubble: .ready
        }
    }
}

private struct CommentComposerHarnessScreen: View {
    let scenario: CommentComposerScenario
    let acts: Bool

    @State private var detail: FeedItemDetailModel
    @State private var toasts = ToastCenter()

    init(scenario: CommentComposerScenario, acts: Bool) {
        self.scenario = scenario
        self.acts = acts
        _detail = State(initialValue: .preview(scenario.previewMode, clock: ContinuousClock()))
    }

    private static let thesis = "Small add to the position. Earnings next week; I want more before. 👍🏽"

    private var thesisText: String? {
        switch scenario {
        case .menu, .draft, .thesisSheet, .thesisBubble: Self.thesis
        default: nil
        }
    }

    @ViewBuilder private var stage: some View {
        if scenario == .thesisBubble {
            AttachmentBubble(
                commentID: "c1",
                attachment: .thesisQuote(
                    .init(kind: .thesisQuote, proposalId: "p1", text: Self.thesis, proposerName: "Mia"))
            )
            .padding(MonacoTheme.Space.gutter)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        } else if scenario == .thesisSheet {
            ThesisQuoteSheet(thesis: Self.thesis) { detail.comments.pick($0, toasts: toasts) }
        } else {
            Color.clear.frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    var body: some View {
        stage
            .monacoCanvas()
            .safeAreaInset(edge: .bottom, spacing: 0) {
                CommentComposerBar(model: detail.comments, menuItems: menuItems, thesis: thesisText)
            }
            .environment(toasts)
            .monacoToastCenter(toasts)
            .task { await run() }
    }

    private var menuItems: [ComposerMenuItem] {
        guard scenario == .menu || scenario == .draft || scenario == .menuSale else { return [] }
        let model = detail.comments
        let toasts = toasts
        return [
            ComposerMenuItem(
                id: "poll", title: "Poll", icon: .pollCheck, isAvailable: { _ in true },
                action: { model.pick(.thesis(start: 0, end: 5), toasts: toasts) })
        ]
    }

    private func run() async {
        let model = detail.comments
        await model.load()
        switch scenario {
        case .docked, .menu, .menuSale, .thesisSheet, .thesisBubble:
            break
        case .draft:
            _ = model.pick(.thesis(start: 0, end: 5))
        case .sheetComment:
            if let row = model.rows.first { model.presentReplySheet(.comment(row)) }
        case .sheetPost:
            model.presentReplySheet(.post(Self.post))
        case .sending, .failed:
            guard acts else { break }
            model.edit("Count me in if we cap it at $25.")
            _ = await model.post(CommentDraft(text: model.draft))
        }
    }

    private static var post: Components.Schemas.FeedItem {
        var item = Components.Schemas.FeedItem.samples[0]
        item.kind = "proposal"
        item.actorName = "Logan"
        item.cabalName = "Tech Titans"
        item.symbol = "AAPLx"
        item.action = nil
        item.usdcMicros = 480_000_000
        item.changeBps = 670
        return item
    }
}
#endif
