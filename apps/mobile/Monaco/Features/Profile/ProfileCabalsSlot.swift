import MonacoAPI
import MonacoCore
import SwiftUI

enum ProfileCabalsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileCabals()
    }
}

private struct ProfileCabals: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: MonacoCore.CabalsTabModel?

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Your cabals")
                .padding(.horizontal, MonacoTheme.Space.m)
            content
        }
        .onAppear {
            let model = preparedModel()
            refresh?.register("profile-cabals") { await model.load() }
            Task { await model.load() }
        }
        .onChange(of: model?.failureTick) { _, _ in
            guard case .loaded = model?.state, let error = model?.lastError else { return }
            toasts.show(error)
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
            EmptyState(title: "Couldn't load your cabals.", actionTitle: "Try again") {
                Task { await model?.load() }
            }
            .accessibilityIdentifier("profile-cabals-retry")
        case .loaded(let cabals) where cabals.isEmpty:
            EmptyState(title: "No cabals yet", message: "Start a cabal or join one from the Cabals tab.")
                .accessibilityIdentifier("profile-cabals-empty")
        case .loaded(let cabals):
            MonacoGroupedList {
                ForEach(cabals, id: \.id) { cabal in
                    CabalPortfolioRow(cabal: cabal, isLast: cabal.id == cabals.last?.id)
                }
            }
            Text("Pot values show up here soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .padding(.horizontal, MonacoTheme.Space.m)
                .accessibilityIdentifier("profile-cabals-coming")
        }
    }

    private func preparedModel() -> MonacoCore.CabalsTabModel {
        if let model { return model }
        let created = MonacoCore.CabalsTabModel(api: environment.api)
        model = created
        return created
    }
}
