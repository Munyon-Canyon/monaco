import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalValueChartSlot: CabalSection {
    static let isLive = true
    static let ranges: [LeaderboardRange] = [.oneDay, .oneWeek, .oneMonth, .all]

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
    @State private var refreshedTick = 0

    var body: some View {
        CabalInkBand {
            Rectangle()
                .fill(MonacoTheme.onHeroHairline)
                .frame(height: 1)
                .padding(.bottom, MonacoTheme.Space.s)
                .accessibilityHidden(true)
            content
        }
        .task(id: retry.tick) {
            let model = prepared()
            let pulled = retry.tick > refreshedTick
            refreshedTick = retry.tick
            await withTaskGroup(of: Void.self) { group in
                group.addTask {
                    if pulled { await model.refresh() } else { await model.load() }
                }
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
                skeleton
            case .failed:
                MonacoErrorRow(thing: "the pot's history", identifier: "cabal-value-chart-failed", inset: false) {
                    Task { await model.load() }
                }
            case .loaded:
                loaded(model)
            }
        } else {
            skeleton
        }
    }

    @ViewBuilder private var skeleton: some View {
        CurveReadoutLine(readout: nil)
        SkeletonBlock(height: 160, radius: 12)
            .accessibilityIdentifier("cabal-value-chart-loading")
        MonacoRangeChipsSkeleton(ranges: CabalValueChartSlot.ranges)
    }

    @ViewBuilder private func loaded(_ model: ValueChartModel) -> some View {
        CurveReadoutLine(readout: model.curve.flatMap { curve in selection.flatMap { curve.readout(at: $0) } })
        if let curve = model.curve, curve.hasEnoughHistory {
            CurveScrubChart(
                curve: curve, range: model.shownRange ?? model.range, selection: $selection,
                identifier: "cabal-value-chart")
        } else {
            CabalInkCaption(model.range.shortHistoryLine, id: "cabal-value-chart-short")
                .frame(height: 160, alignment: .center)
        }
        MonacoRangeChips(
            ranges: model.ranges, selection: model.range, identifierPrefix: "cabal-value-chart"
        ) { range in
            selection = nil
            Task { await model.select(range) }
        }
    }

    private func prepared() -> ValueChartModel {
        if let model { return model }
        let created = ValueChartModel(
            subjects: [.cabal(id: cabalID)], ranges: CabalValueChartSlot.ranges, range: .oneMonth,
            api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}
