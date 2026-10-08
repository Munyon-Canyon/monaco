import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalValueChartSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalValueChart(cabalID: context.cabalID)
    }
}

private struct CabalValueChart: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalRetry) private var retry
    @State private var model: ValueChartModel?
    @State private var selection: Int?

    var body: some View {
        CabalInkBand {
            Rectangle()
                .fill(MonacoTheme.onHeroHairline)
                .frame(height: 1)
                .padding(.vertical, MonacoTheme.Space.s)
                .accessibilityHidden(true)
            content
        }
        .task(id: retry.tick) {
            let model = prepared()
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
        if let model {
            switch model.state {
            case .idle, .loading:
                SkeletonBlock(height: 160, radius: 12)
                    .accessibilityIdentifier("cabal-value-chart-loading")
            case .failed:
                MonacoErrorRow(thing: "the pot's history", identifier: "cabal-value-chart-failed", onHero: true) {
                    Task { await model.load() }
                }
            case .loaded:
                loaded(model)
            }
        } else {
            SkeletonBlock(height: 160, radius: 12)
        }
    }

    @ViewBuilder private func loaded(_ model: ValueChartModel) -> some View {
        if let curve = model.curve, curve.hasEnoughHistory {
            CurveReadoutLine(readout: selection.flatMap { curve.readout(at: $0) }, onInk: true)
            CurveScrubChart(
                curve: curve, range: model.range, onInk: true, selection: $selection,
                identifier: "cabal-value-chart")
        } else {
            CabalInkCaption(model.range.shortHistoryLine, id: "cabal-value-chart-short")
        }
        MonacoRangeChips(
            ranges: model.ranges, selection: model.range, onInk: true, identifierPrefix: "cabal-value-chart"
        ) { range in
            selection = nil
            Task { await model.select(range) }
        }
    }

    private func prepared() -> ValueChartModel {
        if let model { return model }
        let created = ValueChartModel(
            subjects: [.cabal(id: cabalID)], ranges: CabalValueHistoryModel.ranges, range: .oneMonth,
            api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}
