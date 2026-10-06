import MonacoAPI
import MonacoCore
import SwiftUI

enum UserProfileSharedCabalsSlot: UserProfileSection {
    static let isLive = true

    static func body(for context: UserProfileContext) -> some View {
        UserProfileSharedCabals(userID: context.userID, displayName: context.preview?.displayName)
    }
}

private struct UserProfileSharedCabals: View {
    let userID: String
    let displayName: String?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: SharedCabalsModel?

    var body: some View {
        if userID != environment.viewer?.userID {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("Cabals you share")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                content
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityIdentifier("user-profile-shared")
            .task(id: userID) {
                let model = prepared()
                refresh?.register("user-profile-shared") { await model.load() }
                await withTaskGroup(of: Void.self) { group in
                    group.addTask { await model.load() }
                    group.addTask { await model.observe() }
                }
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
            .onChange(of: model?.toast) { _, message in
                guard let message else { return }
                toasts.current = MonacoToast(message: message)
                model?.dismissToast()
            }
        }
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            BoardRowSkeleton(rows: 2)
                .accessibilityElement()
                .accessibilityLabel("Loading shared cabals")
                .accessibilityIdentifier("user-profile-shared-loading")
        case .failed:
            EmptyState(title: "Couldn't load shared cabals.", actionTitle: "Try again") {
                Task { await model?.load() }
            }
            .accessibilityIdentifier("user-profile-shared-retry")
        case .loaded(let summary) where summary.isEmpty:
            EmptyState(
                title: "No cabals in common",
                message: SharedCabalsSummary.emptyLine(displayName: displayName)
            )
            .accessibilityIdentifier("user-profile-shared-empty")
        case .loaded(let summary):
            MonacoGroupedList {
                ForEach(summary.rows) { row in
                    SharedCabalRow(row: row, isLast: row.id == summary.rows.last?.id)
                }
            }
        }
    }

    private func prepared() -> SharedCabalsModel {
        if let model { return model }
        let created = SharedCabalsModel(userID: userID, api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}

private struct SharedCabalRow: View {
    let row: SharedCabalsSummary.Row
    let isLast: Bool

    var body: some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: row.id))) {
            MonacoRow(title: row.name, subtitle: "Pot \(row.potValue)", isLast: isLast) {
                CabalMark(groupId: row.id, name: row.name, size: 40, pictureUrl: row.pictureURL)
            } trailing: {
                if let bps = row.returnBps {
                    PercentText(basisPoints: bps, style: .row)
                } else {
                    PercentText(percentReturn: nil, style: .row)
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("shared-cabal-row-\(row.id)")
    }
}
