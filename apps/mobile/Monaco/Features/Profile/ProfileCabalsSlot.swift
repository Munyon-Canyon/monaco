import MonacoAPI
import MonacoCore
import SwiftUI

enum ProfileCabalsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileCabals()
    }
}

struct ProfileCabals: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: PortfolioModel?

    init(model: PortfolioModel? = nil) {
        _model = State(initialValue: model)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Your cabals")
                .padding(.horizontal, MonacoTheme.Space.m)
            content
        }
        .task {
            let model = preparedModel()
            refresh?.register("profile-cabals") { await model.load() }
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

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            BoardRowSkeleton(rows: 3)
                .accessibilityElement()
                .accessibilityLabel("Loading your cabals")
                .accessibilityIdentifier("profile-cabals-loading")
        case .failed:
            MonacoErrorRow(thing: "your cabals", identifier: "profile-cabals-retry") {
                Task { await model?.load() }
            }
        case .loaded(let summary) where summary.isEmpty:
            EmptyState(
                title: "No cabals yet", message: "Start a cabal or join one from the Cabals tab.", actionTitle: nil
            )
            .accessibilityIdentifier("profile-cabals-empty")
        case .loaded(let summary):
            MonacoGroupedList {
                ForEach(summary.rows) { row in
                    CabalPortfolioRow(row: row, isLast: row.id == summary.rows.last?.id)
                }
            }
        }
    }

    private func preparedModel() -> PortfolioModel {
        if let model { return model }
        let created = PortfolioModel(api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}
