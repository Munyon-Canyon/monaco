import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalsValueChartSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        CabalsValueChart()
    }

    static func shows(_ phase: CabalValueHistoryModel.Phase) -> Bool {
        phase != .hidden
    }
}

private struct CabalsValueChart: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: CabalValueHistoryModel?

    private var phase: CabalValueHistoryModel.Phase { model?.phase ?? .loading }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if CabalsValueChartSlot.shows(phase) {
                MonacoSectionHeader("Your cabals' return")
                content
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-value-chart")
        .task {
            let model = preparedModel()
            refresh?.register("cabals-value-chart") { await model.load() }
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
        switch phase {
        case .loading, .hidden:
            SkeletonBlock(height: 160, radius: 12)
                .accessibilityIdentifier("cabals-value-chart-loading")
        case .failed:
            MonacoErrorRow(thing: "your cabals' return", identifier: "cabals-value-chart-error") {
                Task { await model?.load() }
            }
        case .loaded:
            if let model {
                loaded(model)
            }
        }
    }

    @ViewBuilder private func loaded(_ model: CabalValueHistoryModel) -> some View {
        if model.hasEnoughHistory {
            CabalLinesChart(
                lines: model.lines.filter(\.curve.hasEnoughHistory).map {
                    .init(id: $0.id, name: $0.name, points: $0.curve.navPoints)
                },
                range: model.range, identifier: "cabals-value-chart-lines")
        } else {
            MonacoRule()
                .padding(.vertical, MonacoTheme.Space.m)
            Text(model.range.shortHistoryLine)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("cabals-value-chart-short")
        }
        MonacoRangeChips(
            ranges: model.ranges, selection: model.range, identifierPrefix: "cabals-value-chart"
        ) { range in
            Task { await model.select(range) }
        }
    }

    private func preparedModel() -> CabalValueHistoryModel {
        if let model { return model }
        let created = CabalValueHistoryModel(api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}
