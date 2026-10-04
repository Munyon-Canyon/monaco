import MonacoAPI
import MonacoCore
import SwiftUI

enum ProfileStatsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileStats()
    }

    static func cabalsCount(_ state: LoadState<[Components.Schemas.MyCabal]>) -> String? {
        guard case .loaded(let cabals) = state else { return nil }
        return "\(cabals.count)"
    }
}

private struct ProfileStats: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: MonacoCore.CabalsTabModel?

    var body: some View {
        content
            .onAppear {
                let model = preparedModel()
                refresh?.register("profile-stats") { await model.load() }
                Task { await model.load() }
            }
            .onChange(of: model?.failureTick) { _, _ in
                guard case .loaded = model?.state, let error = model?.lastError else { return }
                toasts.show(error)
            }
    }

    @ViewBuilder private var content: some View {
        let state = model?.state ?? .loading
        if case .failed = state {
            EmptyState(title: "Couldn't load your stats.", actionTitle: "Try again") {
                Task { await model?.load() }
            }
            .accessibilityIdentifier("profile-stats-retry")
        } else if let count = ProfileStatsSlot.cabalsCount(state) {
            band(cabals: count)
        } else {
            skeleton
        }
    }

    private func band(cabals: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            columns {
                ProfileStatColumn(value: nil, label: "In cabals")
                    .accessibilityIdentifier("profile-stat-in-cabals")
                ProfileStatColumn(value: nil, label: "All time")
                    .accessibilityIdentifier("profile-stat-all-time")
                ProfileStatColumn(value: cabals, label: "Cabals")
                    .accessibilityIdentifier("profile-stat-cabals")
            }
            Text("Your totals show up here soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .padding(.horizontal, MonacoTheme.Space.m)
                .accessibilityIdentifier("profile-stats-coming")
        }
    }

    private var skeleton: some View {
        columns {
            ForEach(0..<3, id: \.self) { _ in
                VStack(spacing: MonacoTheme.Space.xs) {
                    SkeletonBlock(width: 48, height: 16)
                    SkeletonBlock(width: 64, height: 12)
                }
                .frame(maxWidth: .infinity)
            }
        }
        .accessibilityElement()
        .accessibilityLabel("Loading your stats")
        .accessibilityIdentifier("profile-stats-loading")
    }

    private func columns(@ViewBuilder _ cells: () -> some View) -> some View {
        VStack(spacing: 0) {
            MonacoRule()
            HStack(spacing: 0) { cells() }
                .padding(.vertical, MonacoTheme.Space.m)
            MonacoRule()
        }
        .padding(.horizontal, MonacoTheme.Space.m)
    }

    private func preparedModel() -> MonacoCore.CabalsTabModel {
        if let model { return model }
        let created = MonacoCore.CabalsTabModel(api: environment.api)
        model = created
        return created
    }
}

private struct ProfileStatColumn: View {
    let value: String?
    let label: String

    var body: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            Text(value ?? "—")
                .font(MonacoTheme.Typo.dataStrong)
                .foregroundStyle(value == nil ? MonacoTheme.muted : MonacoTheme.ink)
                .accessibilityLabel(value ?? "Not available yet")
            Text(label)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        }
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .combine)
    }
}
