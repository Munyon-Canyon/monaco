import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalsValueChartSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        CabalsValueChart()
    }

    static func shows(_ state: LoadState<[Components.Schemas.MyCabal]>) -> Bool {
        if case .loaded(let cabals) = state { return !cabals.isEmpty }
        return false
    }
}

private struct CabalsValueChart: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: MonacoCore.CabalsTabModel?

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if let model, CabalsValueChartSlot.shows(model.state) {
                MonacoSectionHeader("Your cabals' return")
                Text("Your cabals' return shows up here soon.")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityIdentifier("cabals-value-chart-coming")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-value-chart")
        .onAppear {
            let model = preparedModel()
            refresh?.register("cabals-value-chart") { await model.load() }
            Task { await model.load() }
        }
    }

    private func preparedModel() -> MonacoCore.CabalsTabModel {
        if let model { return model }
        let created = MonacoCore.CabalsTabModel(api: environment.api)
        model = created
        return created
    }
}
