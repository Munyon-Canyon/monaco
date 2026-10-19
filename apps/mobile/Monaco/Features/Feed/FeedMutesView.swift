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
            EmptyState(
                title: "Nothing muted",
                message: "Press and hold a post in your feed to mute its cabal, stock or person. They'll show up here.",
                systemImage: MonacoIcon.feed.symbol
            )
            .accessibilityIdentifier("feed-muted-empty")
        case .failed:
            MonacoErrorRow(thing: "your mutes", identifier: "feed-muted-error") {
                Task { await model.load() }
            }
        case .loaded(let mutes):
            LazyVStack(spacing: 0) {
                ForEach(Array(mutes.enumerated()), id: \.element.id) { index, mute in
                    row(mute, isLast: index == mutes.count - 1)
                }
            }
        }
    }

    private func row(_ mute: Components.Schemas.FeedMute, isLast: Bool) -> some View {
        MonacoRow(
            title: FeedMutesModel.label(mute), isLast: isLast, trailingIsInteractive: true,
            leading: {
                Image(monaco: Self.icon(for: mute))
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.brand)
                    .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
                    .background(Circle().fill(MonacoTheme.brandSoft))
                    .accessibilityHidden(true)
            },
            trailing: {
                Button("Unmute") {
                    Task {
                        guard let message = await model.unmute(mute) else { return }
                        toasts.show(success: message)
                    }
                }
                .buttonStyle(.monacoCompact)
                .accessibilityIdentifier("feed-unmute-\(mute.id)")
            }
        )
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("feed-muted-row-\(mute.id)")
    }

    private static func icon(for mute: Components.Schemas.FeedMute) -> MonacoIcon {
        switch FeedMuteTarget.Kind(rawValue: mute.targetType) {
        case .user: .profile
        case .cabal: .cabals
        case .asset: .markets
        case .kind, .item, nil: .feed
        }
    }
}
