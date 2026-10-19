import Charts
import MonacoCore
import SwiftUI

enum MonacoChartStyle: String, CaseIterable, Hashable {
    case candles
    case line

    var symbol: String {
        switch self {
        case .candles: "chart.bar.xaxis"
        case .line: "chart.xyaxis.line"
        }
    }

    var accessibilityName: String {
        switch self {
        case .candles: "Candles"
        case .line: "Line"
        }
    }
}

struct MonacoCandleChart: View {
    let bars: [PlottedBar]
    let intervalSeconds: Int
    var style: MonacoChartStyle
    var tint: Color
    var lastPrice: Double?
    var isLive: Bool = false
    var fixedDomain: ClosedRange<Double>?
    var baseline: Double?
    var tagTint: Color?
    var showsLastPriceRule = true
    var showsEndDot = false
    var selectionStyle: MonacoChartSelectionStyle = .axisTag
    var showsDateRange = false
    var resetKey: AnyHashable
    var hasOlder: Bool
    var retryToken = 0
    var initialVisibleBars: Int?
    var valueText: ((Double) -> String)?
    var tooltipAmount: ((Int) -> String?)?
    var gutter: CGFloat = 56
    var height: CGFloat = 216
    @Binding var selection: Int?
    var onNeedOlder: () -> Void
    var summary: String
    var nearestIndex: (Date) -> Int?
    var describeBar: (Int) -> String
    var accessibilityIdentifier: String = "monaco-candle-chart"

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.scrubSelectionPersists) private var selectionPersists
    @State private var window: ChartWindow
    @State private var scrollRevision = 0
    @State private var plot = CGRect.zero
    @State private var pinchStart: PinchStart?
    @State private var isHolding = false
    @State private var requestedBefore: Date?
    @State private var appeared = false
    @State private var chartWidth: CGFloat = 0
    @State private var store: MonacoBarStore

    static let axisHeight: CGFloat = 24

    private struct PinchStart {
        let window: ChartWindow
        let anchorDate: Date
    }

    init(
        bars: [PlottedBar],
        intervalSeconds: Int,
        style: MonacoChartStyle,
        tint: Color,
        lastPrice: Double?,
        isLive: Bool = false,
        fixedDomain: ClosedRange<Double>? = nil,
        baseline: Double? = nil,
        tagTint: Color? = nil,
        showsLastPriceRule: Bool = true,
        showsEndDot: Bool = false,
        selectionStyle: MonacoChartSelectionStyle = .axisTag,
        showsDateRange: Bool = false,
        resetKey: AnyHashable,
        hasOlder: Bool,
        retryToken: Int = 0,
        initialVisibleBars: Int? = nil,
        valueText: ((Double) -> String)? = nil,
        tooltipAmount: ((Int) -> String?)? = nil,
        gutter: CGFloat = 56,
        height: CGFloat = 216,
        selection: Binding<Int?>,
        onNeedOlder: @escaping () -> Void,
        summary: String,
        nearestIndex: @escaping (Date) -> Int?,
        describeBar: @escaping (Int) -> String,
        accessibilityIdentifier: String = "monaco-candle-chart"
    ) {
        self.bars = bars
        self.intervalSeconds = intervalSeconds
        self.style = style
        self.tint = tint
        self.lastPrice = lastPrice
        self.isLive = isLive
        self.fixedDomain = fixedDomain
        self.baseline = baseline
        self.tagTint = tagTint
        self.showsLastPriceRule = showsLastPriceRule
        self.showsEndDot = showsEndDot
        self.selectionStyle = selectionStyle
        self.showsDateRange = showsDateRange
        self.resetKey = resetKey
        self.hasOlder = hasOlder
        self.retryToken = retryToken
        self.initialVisibleBars = initialVisibleBars
        self.valueText = valueText
        self.tooltipAmount = tooltipAmount
        self.gutter = gutter
        self.height = height
        _selection = selection
        self.onNeedOlder = onNeedOlder
        self.summary = summary
        self.nearestIndex = nearestIndex
        self.describeBar = describeBar
        self.accessibilityIdentifier = accessibilityIdentifier
        _store = State(initialValue: MonacoBarStore(bars))
        _window = State(
            initialValue: ChartWindow.initial(
                bars: bars, intervalSeconds: intervalSeconds,
                visibleBars: initialVisibleBars ?? ChartWindow.defaultVisibleBars))
    }

    private var domain: ClosedRange<Double> {
        if let fixedDomain { return fixedDomain }
        return window.valueDomain(of: bars, slack: TimeInterval(intervalSeconds) / 2, closesOnly: style == .line)
    }

    private var xDomain: ClosedRange<Date> {
        guard let first = bars.first?.date, let last = bars.last?.date else {
            return window.scrollDate...window.endDate
        }
        let step = TimeInterval(intervalSeconds)
        return first.addingTimeInterval(-step)...last.addingTimeInterval(step)
    }

    private var isAtLatest: Bool {
        window.isAtLatest(bars: bars, intervalSeconds: intervalSeconds)
    }

    private var scrollPosition: Binding<Date> {
        Binding(
            get: { window.scrollDate },
            set: { newValue in
                guard newValue != window.scrollDate else { return }
                window.scrollDate = newValue
            }
        )
    }

    var body: some View {
        VStack(spacing: 0) {
            plotArea
            if let dates = visibleDates {
                MonacoChartDateRow(first: dates.first, last: dates.last)
            }
        }
        .opacity(appeared ? 1 : 0)
        .onAppear {
            withAnimation(reduceMotion ? nil : .easeOut(duration: 0.2)) { appeared = true }
            checkOlder()
        }
        .onChange(of: bars) { _, new in store = MonacoBarStore(new) }
        .onChange(of: retryToken) { _, _ in
            requestedBefore = nil
            checkOlder()
        }
        .onChange(of: resetKey) { _, _ in reset() }
        .onChange(of: bars.last?.date) { old, _ in followLatest(previousLast: old) }
        .onChange(of: bars.first?.date) { _, _ in requestedBefore = nil }
        .onChange(of: window) { _, _ in checkOlder() }
    }

    private var plotArea: some View {
        ZStack(alignment: .topLeading) {
            MonacoCandleSeries(
                intervalSeconds: intervalSeconds,
                style: style,
                domain: domain,
                xDomain: xDomain,
                visibleSeconds: window.visibleSeconds,
                scrollRevision: scrollRevision,
                scrollLocked: pinchStart != nil || isHolding,
                scrollPosition: scrollPosition,
                plot: $plot
            )
            .equatable()
            .padding(.bottom, style == .candles ? Self.axisHeight : 0)
            .padding(.trailing, style == .line ? gutter : 0)
            if plot.width > 0 {
                MonacoCandleCanvas(
                    store: store,
                    window: window,
                    domain: domain,
                    plot: plot,
                    intervalSeconds: intervalSeconds,
                    style: style,
                    tint: tint,
                    selection: selection
                )
                MonacoCandleOverlay(
                    store: store,
                    window: window,
                    domain: domain,
                    plot: plot,
                    chartWidth: chartWidth,
                    style: style,
                    tint: tint,
                    lastPrice: lastPrice,
                    baseline: baseline,
                    tagTint: tagTint,
                    showsLastPriceRule: showsLastPriceRule,
                    showsEndDot: showsEndDot,
                    selectionStyle: selectionStyle,
                    isLive: isLive,
                    isAtLatest: isAtLatest,
                    selection: selection,
                    intervalSeconds: intervalSeconds,
                    valueText: valueText,
                    tooltipAmount: tooltipAmount,
                    onJump: jumpToLatest
                )
            }
        }
        .onGeometryChange(for: CGFloat.self) {
            $0.size.width
        } action: {
            chartWidth = $0
        }
        .frame(height: height)
        .simultaneousGesture(pinch)
        .background(
            MonacoChartHold(onBegan: beginHold, onMoved: moveCrosshair, onEnded: endHold)
        )
        .sensoryFeedback(trigger: selection) { _, new in new == nil ? nil : .selection }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(summary)
        .accessibilityValue(voiceOverValue)
        .accessibilityAdjustableAction(step)
        .accessibilityScrollAction { _ in selection = nil }
        .accessibilityIdentifier(accessibilityIdentifier)
    }
}

extension MonacoCandleChart {
    private func reset() {
        window = ChartWindow.initial(
            bars: bars, intervalSeconds: intervalSeconds,
            visibleBars: initialVisibleBars ?? ChartWindow.defaultVisibleBars)
        scrollRevision += 1
        requestedBefore = nil
        selection = nil
    }

    private func followLatest(previousLast: Date?) {
        guard let last = bars.last?.date, let previousLast, last > previousLast else { return }
        let wasPinned = window.endDate >= previousLast.addingTimeInterval(TimeInterval(intervalSeconds) / 2)
        guard wasPinned else { return }
        window = window.pinnedToLatest(of: bars, intervalSeconds: intervalSeconds)
        scrollRevision += 1
    }

    private func checkOlder() {
        let needs = window.needsOlder(bars: bars, intervalSeconds: intervalSeconds, hasOlder: hasOlder)
        guard needs else {
            requestedBefore = nil
            return
        }
        guard requestedBefore == nil, let first = bars.first?.date else { return }
        requestedBefore = first
        onNeedOlder()
    }

    private func jumpToLatest() {
        Haptics.selection()
        let target = window.pinnedToLatest(of: bars, intervalSeconds: intervalSeconds)
        withAnimation(reduceMotion ? nil : .easeInOut(duration: 0.35)) {
            window = target
            scrollRevision += 1
        }
    }

    private var visibleDates: (first: String, last: String)? {
        guard showsDateRange else { return nil }
        return MonacoChartDateRow.labels(bars: bars, window: window, intervalSeconds: intervalSeconds)
    }

    private func date(atX x: CGFloat) -> Date {
        guard plot.width > 0 else { return window.scrollDate }
        let fraction = Double((x - plot.minX) / plot.width)
        return window.scrollDate.addingTimeInterval(fraction * window.visibleSeconds)
    }

    private var pinch: some Gesture {
        MagnifyGesture()
            .onChanged { value in
                let start = pinchStart ?? beginPinch(anchor: value.startAnchor.x)
                pinchStart = start
                let zoomed = start.window.zoomed(
                    scale: value.magnification,
                    anchorDate: start.anchorDate,
                    intervalSeconds: intervalSeconds,
                    maxBars: min(max(bars.count, ChartWindow.minimumBars), ChartWindow.maximumBars)
                )
                .clamped(to: bars, intervalSeconds: intervalSeconds)
                guard zoomed != window else { return }
                window = zoomed
                scrollRevision += 1
            }
            .onEnded { _ in pinchStart = nil }
    }

    private func beginPinch(anchor: CGFloat) -> PinchStart {
        let fraction = min(max(anchor, 0), 1)
        let anchorX = fraction * chartWidth
        return PinchStart(window: window, anchorDate: date(atX: anchorX))
    }

    private func beginHold(at point: CGPoint) {
        isHolding = true
        moveCrosshair(to: point)
    }

    private func moveCrosshair(to location: CGPoint) {
        selection = nearestIndex(date(atX: location.x))
    }

    private func endHold() {
        guard isHolding else { return }
        isHolding = false
        if !selectionPersists { selection = nil }
    }

    private func step(_ direction: AccessibilityAdjustmentDirection) {
        selection = MonacoScrubStep.next(from: selection, forward: direction == .increment, count: bars.count)
    }

    private var voiceOverValue: String {
        guard !bars.isEmpty else { return "No price history" }
        return describeBar(selection ?? bars.count - 1)
    }
}

struct MonacoChartDateRow: View {
    let first: String
    let last: String

    static func labels(
        bars: [PlottedBar], window: ChartWindow, intervalSeconds: Int,
        locale: Locale = .autoupdatingCurrent, timeZone: TimeZone = .autoupdatingCurrent
    ) -> (first: String, last: String)? {
        let range = window.visibleRange(of: bars)
        guard let first = range.first, let last = range.last else { return nil }
        func text(_ date: Date) -> String {
            ChartCrosshairLabel.text(
                date, visibleSeconds: window.visibleSeconds, intervalSeconds: intervalSeconds, locale: locale,
                timeZone: timeZone)
        }
        func weekdayText(_ date: Date) -> String {
            ChartScrubLabel.caption(for: date, range: .oneDay, locale: locale, timeZone: timeZone)
        }
        let dates = (bars[first].date, bars[last].date)
        guard text(dates.0) == text(dates.1), dates.0 != dates.1 else { return (text(dates.0), text(dates.1)) }
        return (weekdayText(dates.0), weekdayText(dates.1))
    }

    var body: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
            Text(first)
            Spacer(minLength: 0)
            Text(last).multilineTextAlignment(.trailing)
        }
        .font(MonacoTheme.Typo.caption)
        .foregroundStyle(MonacoTheme.secondaryText)
        .padding(.top, MonacoTheme.Space.xs)
        .padding(.bottom, MonacoTheme.Space.s)
        .accessibilityHidden(true)
    }
}
