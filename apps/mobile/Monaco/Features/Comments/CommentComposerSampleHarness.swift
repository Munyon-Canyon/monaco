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

    var previewMode: FeedItemDetailModel.PreviewMode {
        switch self {
        case .sending: .sending
        case .failed: .failed
        case .docked, .sheetComment, .sheetPost: .ready
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

    var body: some View {
        Color.clear
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
            .safeAreaInset(edge: .bottom, spacing: 0) {
                CommentComposerBar(model: detail.comments, onPlus: {}, onGif: {})
            }
            .environment(toasts)
            .monacoToastCenter(toasts)
            .task { await run() }
    }

    private func run() async {
        let model = detail.comments
        await model.load()
        switch scenario {
        case .docked:
            break
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
