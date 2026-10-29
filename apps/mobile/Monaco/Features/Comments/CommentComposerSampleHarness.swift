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
    case stockSheet
    case stockBubble
    case poll

    var previewMode: FeedItemDetailModel.PreviewMode {
        switch self {
        case .sending: .sending
        case .failed: .failed
        case .poll: .poll
        case .docked, .sheetComment, .sheetPost, .menu, .draft, .menuSale, .thesisSheet, .thesisBubble,
            .stockSheet, .stockBubble:
            .ready
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

    private static let stockSearch: StockSearch = { _ in
        [
            Components.Schemas.AssetSummary(
                id: "01890a5d-ac96-774b-bcce-b302099a8070",
                symbol: "AAPLx", displayName: "Apple", issuer: .xstocks, kind: .equity, logoUrl: nil,
                priceMicros: 180_000_000, priceAsOf: Date(timeIntervalSince1970: 0), changeBps: -80,
                sparklineMicros: [181_000_000, 180_000_000], quotable: true, session: .open, tradable: true)
        ]
    }

    @ViewBuilder private var stage: some View {
        if scenario == .stockBubble {
            AttachmentBubble(
                commentID: "c2",
                attachment: .stock(
                    .init(
                        kind: .stock, assetId: "01890a5d-ac96-774b-bcce-b302099a8070", symbol: "AAPL",
                        name: "Apple", logoUrl: "", isPreIpo: false, priceMicros: 180_000_000, changeBps: -80))
            )
            .padding(MonacoTheme.Space.gutter)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        } else if scenario == .stockSheet {
            StockShareSheet(search: Self.stockSearch) { detail.comments.pick($0, toasts: toasts) }
        } else if scenario == .thesisBubble {
            AttachmentBubble(
                commentID: "c1",
                attachment: .thesisQuote(
                    .init(kind: .thesisQuote, proposalId: "p1", text: Self.thesis, proposerName: "Mia"))
            )
            .padding(MonacoTheme.Space.gutter)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        } else if scenario == .thesisSheet {
            ThesisQuoteSheet(thesis: Self.thesis) { detail.comments.pick($0, toasts: toasts) }
        } else if scenario == .sheetPost {
            NavigationStack {
                FeedView(makeModel: { _ in .preview(.items, clock: ContinuousClock()) })
            }
        } else {
            thread.frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    var body: some View {
        stage
            .monacoCanvas()
            .safeAreaInset(edge: .bottom, spacing: 0) {
                CommentComposerBar(
                    model: detail.comments, menuItems: [], thesis: thesisText, offersPoll: offersPoll)
            }
            .environment(\.pollVoting, PreviewPollVoting())
            .environment(\.stockSearch, Self.stockSearch)
            .environment(toasts)
            .monacoToastCenter(toasts)
            .task { await run() }
    }

    private var offersPoll: Bool { [.menu, .draft, .menuSale, .poll].contains(scenario) }

    @ViewBuilder private var thread: some View {
        if scenario == .poll {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    ForEach(detail.comments.rows) { row in
                        if let attachment = row.comment.attachment {
                            AttachmentBubble(commentID: row.id, attachment: attachment)
                        }
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(MonacoTheme.Space.gutter)
            }
        } else {
            Color.clear
        }
    }

    private func run() async {
        let model = detail.comments
        await model.load()
        switch scenario {
        case .docked, .menu, .menuSale, .thesisSheet, .thesisBubble, .stockSheet, .stockBubble, .poll:
            break
        case .draft:
            _ = model.pick(.thesis(start: 0, end: 5))
        case .sheetComment:
            if let row = model.rows.first { model.presentReplySheet(.comment(row)) }
        case .sheetPost:
            model.edit("Nice call, that one paid off.")
            model.presentReplySheet(.post(Self.post))
        case .sending, .failed:
            guard acts else { break }
            model.edit("Count me in if we cap it at $25.")
            _ = await model.post(CommentDraft(text: model.draft))
        }
    }

    private static var post: Components.Schemas.FeedItem { .results[1] }
}
#endif
