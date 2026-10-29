#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

final class FeedPostSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-feedPostHarness") else { return nil }
        let raw = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "ready"
        let acts = arguments.contains(SampleHarnessRegistry.actArgument)
        return AnyView(
            FeedPostHarnessScreen(mode: FeedItemDetailModel.PreviewMode(rawValue: raw) ?? .ready, acts: acts))
    }
}

private struct FeedPostHarnessScreen: View {
    let mode: FeedItemDetailModel.PreviewMode
    let acts: Bool

    @State private var model: FeedItemDetailModel
    @State private var path: [AnyAppRoute]

    init(mode: FeedItemDetailModel.PreviewMode, acts: Bool) {
        self.mode = mode
        self.acts = acts
        _model = State(initialValue: .preview(mode, clock: ContinuousClock()))
        _path = State(initialValue: [AnyAppRoute(FeedPostSampleRoute())])
    }

    var body: some View {
        NavigationStack(path: $path) {
            FeedView(makeModel: { _ in .preview(.items, clock: ContinuousClock()) })
                .navigationDestination(for: AnyAppRoute.self) { route in
                    if route.is(FeedPostSampleRoute.self) {
                        PostScreen(
                            itemID: FeedItemDetailModel.previewItemID, startsAtEnd: mode == .end,
                            makeModel: { _, _, _ in model },
                            makeFeed: { _ in .preview(.items, clock: ContinuousClock()) })
                    } else {
                        route.destination()
                    }
                }
        }
        .task { await act() }
    }

    private func act() async {
        guard acts else { return }
        await model.load()
        try? await Task.sleep(for: .milliseconds(500))
        switch mode {
        case .replying:
            if let row = model.comments.rows.first { model.comments.presentReplySheet(.comment(row)) }
        case .sending, .failed:
            _ = await model.comments.post(CommentDraft(text: "Count me in if we cap it at $25."))
        case .ready, .empty, .unavailable, .poll, .logan, .end:
            break
        }
    }
}

nonisolated private struct FeedPostSampleRoute: AppRoute {
    @MainActor func destination() -> EmptyView { EmptyView() }
}
#endif
