import MonacoCore
import SwiftUI

struct LeaderboardHost<Content: View>: View {
    let board: LeaderboardBoard
    let refreshKey: String
    var reloadID = 0
    @ViewBuilder let content: (LeaderboardLoader) -> Content

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var loader: LeaderboardLoader?

    var body: some View {
        VStack(spacing: 0) {
            if let loader {
                content(loader)
            } else {
                Color.clear.frame(height: 1)
            }
        }
        .task(id: reloadID) {
            let loader = preparedLoader()
            refresh?.register(refreshKey) { await loader.load() }
            await loader.load()
            await loader.observe()
        }
        .onScreenVisibilityChange { loader?.setVisible($0) }
        .onChange(of: loader?.toast) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
            loader?.dismissToast()
        }
    }

    private func preparedLoader() -> LeaderboardLoader {
        if let loader { return loader }
        let created = LeaderboardLoader(board: board, api: environment.api, hints: environment.hints)
        loader = created
        return created
    }
}

struct LeaderboardFreshnessText: View {
    let loader: LeaderboardLoader
    let identifier: String

    var body: some View {
        TimelineView(.periodic(from: .now, by: 30)) { context in
            if let text = loader.freshness(now: context.date) {
                Text(text)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityIdentifier(identifier)
            }
        }
    }
}

struct LeaderboardBoardList<Empty: View, RowContent: View>: View {
    let loader: LeaderboardLoader
    let skeletonRows: Int
    let failureText: String
    let identifier: String
    var showsEmpty = true
    @ViewBuilder let empty: () -> Empty
    @ViewBuilder let rowContent: (LeaderboardRowView, Bool) -> RowContent

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        switch loader.phase {
        case .loading:
            BoardRowSkeleton(rows: skeletonRows)
                .accessibilityIdentifier("\(identifier)-loading")
        case .empty:
            if showsEmpty {
                empty()
            }
        case .failed:
            failure
        case .loaded:
            rows
        }
    }

    private var failure: some View {
        HStack {
            Text(failureText)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
            Spacer()
            Button("Try again") { loader.retry() }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("\(identifier)-retry")
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .accessibilityIdentifier("\(identifier)-error")
    }

    private var rows: some View {
        let rows = loader.rows
        let pinned = loader.pinnedMe
        return MonacoGroupedList {
            ForEach(rows) { row in
                rowContent(row, row.id == rows.last?.id && pinned == nil)
                    .onScrollVisibilityChange(threshold: 0.1) { visible in
                        guard visible, row.id == rows.last?.id, loader.hasMore else { return }
                        Task { await loader.loadMore() }
                    }
            }
            if let pinned {
                rowContent(pinned, true)
                    .accessibilityIdentifier("\(identifier)-me")
            }
            if loader.isLoadingMore {
                BoardRowSkeleton(rows: 1)
            }
        }
        .opacity(loader.isLoading ? 0.5 : 1)
        .animation(reduceMotion ? nil : .snappy, value: rows.map(\.id))
        .animation(.easeInOut(duration: 0.15), value: loader.isLoading)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(identifier)
    }
}
