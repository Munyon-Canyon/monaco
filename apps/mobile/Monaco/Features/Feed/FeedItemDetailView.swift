import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct FeedItemRoute: AppRoute {
    let itemID: String

    @MainActor func destination() -> FeedItemDetailView {
        FeedItemDetailView(itemID: itemID)
    }
}

struct FeedItemDetailView: View {
    let itemID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: FeedItemDetailModel?

    var body: some View {
        ZStack {
            MonacoTheme.canvas.ignoresSafeArea()
            if let model {
                FeedItemDetailContent(model: model)
            }
        }
        .navigationTitle("Post")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-item-detail")
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private func preparedModel() -> FeedItemDetailModel {
        if let model { return model }
        let created = FeedItemDetailModel(
            itemID: itemID, api: environment.api, hints: environment.hints, clock: ContinuousClock())
        model = created
        return created
    }
}

private struct FeedItemDetailContent: View {
    let model: FeedItemDetailModel

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    header
                    CommentThreadView(model: model.comments)
                }
                .padding(.vertical, MonacoTheme.Space.m)
            }
            .scrollDismissesKeyboard(.interactively)
            .refreshable { await model.refresh() }
            .safeAreaInset(edge: .bottom, spacing: 0) {
                CommentComposerBar(model: model.comments)
            }
            .onChange(of: model.comments.lastPostedID) { _, id in
                guard let id else { return }
                withAnimation { proxy.scrollTo(id, anchor: .center) }
            }
        }
        .monacoCanvas()
    }

    @ViewBuilder private var header: some View {
        if let item = model.item {
            FeedItemCell(item: item, opensComments: false)
        } else if model.itemError != nil {
            MonacoErrorRow(thing: "this post", identifier: "feed-item-detail-error") {
                Task { await model.load() }
            }
        } else {
            MonacoRowSkeleton(rows: 1, markShape: .circle)
        }
    }
}
