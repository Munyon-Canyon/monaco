import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalValueChartSlot: CabalSection {
    static let isLive = true
    static let ranges: [LeaderboardRange] = [.oneWeek, .oneMonth, .threeMonths, .sixMonths, .all]

    static func body(for context: CabalContext) -> some View {
        CabalValueChart(cabalID: context.cabalID, topInset: CabalChartMetrics.slotInset)
    }
}

enum CabalChartMetrics {
    static let returnGap: CGFloat = MonacoTheme.Space.s
    static let chartGap: CGFloat = MonacoTheme.Space.sm
    static let periodsGap: CGFloat = MonacoTheme.Space.sm
    static let slotInset: CGFloat = -(SectionStackMetrics.spacing - returnGap)
}

struct CabalValueChart: View {
    let cabalID: String
    let topInset: CGFloat

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalRetry) private var retry
    @Environment(\.cabalPotModel) private var pot
    @State private var model: ValueChartModel?
    @State private var selection: Int?
    @State private var refreshedTick = 0

    private var hasNoPositions: Bool {
        guard let state = pot?.summary?.state else { return false }
        return state != .invested
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            content
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, topInset)
        .frame(maxWidth: .infinity, alignment: .leading)
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
        Text(" ").moneyFont(.row).skeletonBar(width: 140, radius: MonacoTheme.Radius.chip)
        chartSkeleton.padding(.top, CabalChartMetrics.chartGap)
        MonacoRangeChipsSkeleton(ranges: CabalValueChartSlot.ranges).padding(.top, CabalChartMetrics.periodsGap)
    }

    private var chartSkeleton: some View {
        CurveScrubChartFrame(height: CurveScrubChart.smallHeight) {
            SkeletonBlock(height: nil, radius: MonacoTheme.Radius.cardTight)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading the chart")
        .accessibilityIdentifier("cabal-value-chart-loading")
    }

    @ViewBuilder private func loaded(_ model: ValueChartModel) -> some View {
        let shown = model.shownRange ?? model.range
        let curve = model.curve.flatMap { $0.hasEnoughHistory ? $0 : nil }
        returnLine(curve, range: shown)
        if hasNoPositions {
            QuietChart(message: "No positions yet")
                .padding(.top, CabalChartMetrics.chartGap)
        } else {
            plot(curve, range: shown)
                .padding(.top, CabalChartMetrics.chartGap)
            MonacoRangeChips(
                ranges: model.ranges, selection: model.range, identifierPrefix: "cabal-value-chart"
            ) { range in
                selection = nil
                Task { await model.select(range) }
            }
            .padding(.top, CabalChartMetrics.periodsGap)
        }
    }

    private func change(_ curve: ValueCurve?) -> ValueCurve.Change? {
        guard let curve, !hasNoPositions else { return nil }
        guard let selection else { return curve.change(.nav) }
        return curve.change(.nav, to: min(selection, max(curve.plotted(.nav).count - 1, 0)))
    }

    private func returnLine(_ curve: ValueCurve?, range: LeaderboardRange) -> some View {
        let change = change(curve)
        let percent = change?.percentText ?? (hasNoPositions ? "0.00%" : "—")
        let color: Color =
            switch change?.direction ?? .flat {
            case .up: MonacoTheme.profit
            case .down: MonacoTheme.loss
            case .flat: MonacoTheme.secondaryText
            }
        return ViewThatFits(in: .horizontal) {
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.sm) {
                returnParts(percent, color: color, range: range)
            }
            VStack(alignment: .leading, spacing: 0) {
                returnParts(percent, color: color, range: range)
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(PnLSpeech.percent(percent)), \(range.returnPeriodPhrase)")
        .accessibilityValue(percent)
        .accessibilityIdentifier("cabal-pot-all-time")
    }

    @ViewBuilder private func returnParts(_ percent: String, color: Color, range: LeaderboardRange) -> some View {
        Text(percent)
            .moneyFigure(.row)
            .foregroundStyle(color)
            .lineLimit(1)
            .fixedSize()
        Text(range.returnPeriodPhrase)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .lineLimit(1)
    }

    @ViewBuilder private func plot(_ curve: ValueCurve?, range: LeaderboardRange) -> some View {
        if let curve {
            CurveScrubChart(
                curve: curve, plot: .nav, range: range, height: CurveScrubChart.smallHeight, selection: $selection,
                identifier: "cabal-value-chart")
        } else {
            CurveScrubChartFrame(height: CurveScrubChart.smallHeight) {
                QuietChart(message: range.shortHistoryLine)
                    .frame(maxHeight: .infinity)
                    .accessibilityIdentifier("cabal-value-chart-short")
            }
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
