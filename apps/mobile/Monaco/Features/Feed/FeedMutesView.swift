import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct FeedMutesRoute: AppRoute {
    @MainActor func destination() -> FeedMutesView {
        FeedMutesView()
    }
}

struct FeedMutesView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: FeedMutesModel?

    var body: some View {
        ZStack {
            MonacoTheme.canvas.ignoresSafeArea()
            if let model {
                FeedMutesList(model: model)
            }
        }
        .navigationTitle("Muted")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-muted-root")
        .task {
            let model = preparedModel()
            await model.load()
        }
    }

    private func preparedModel() -> FeedMutesModel {
        if let model { return model }
        let created = FeedMutesModel(api: environment.api)
        model = created
        return created
    }
}

struct FeedMutesList: View {
    let model: FeedMutesModel

    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        ScrollView {
            content
        }
        .refreshable { await model.load() }
        .monacoCanvas()
        .onChange(of: model.failureTick) { _, _ in
            guard let error = model.lastError else { return }
            toasts.show(error)
        }
    }

    @ViewBuilder private var content: some View {
        switch model.phase {
        case .loading:
            MonacoRowSkeleton(rows: 3, markShape: .circle)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading your mutes")
                .accessibilityIdentifier("feed-muted-loading")
        case .empty:
            EmptyState(title: "Nothing muted")
                .accessibilityIdentifier("feed-muted-empty")
        case .failed:
            MonacoErrorRow(thing: "your mutes", identifier: "feed-muted-error") {
                Task { await model.load() }
            }
        case .loaded(let mutes):
            LazyVStack(spacing: 0) {
                ForEach(Array(mutes.enumerated()), id: \.element.id) { index, mute in
                    row(mute)
                        .overlay(alignment: .bottom) {
                            if index < mutes.count - 1 { MonacoRule().padding(.leading, MonacoTheme.Space.gutter) }
                        }
                }
            }
        }
    }

    private func row(_ mute: Components.Schemas.FeedMute) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            Text(FeedMutesModel.label(mute))
                .font(MonacoTheme.Typo.rowTitle)
                .foregroundStyle(MonacoTheme.ink)
                .frame(maxWidth: .infinity, alignment: .leading)
            Button("Unmute") {
                Task {
                    guard let message = await model.unmute(mute) else { return }
                    toasts.show(success: message)
                }
            }
            .buttonStyle(.monacoSecondary)
            .accessibilityIdentifier("feed-unmute-\(mute.id)")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-muted-row-\(mute.id)")
    }
}
