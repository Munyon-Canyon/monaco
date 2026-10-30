import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalPotSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalPotModelHost(cabalID: context.cabalID) { model in
            CabalTreasuryLine(model: model, cabalID: context.cabalID)
        }
    }
}

private struct CabalTreasuryLine: View {
    let model: CabalPotModel?
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab

    var body: some View {
        CabalEmptyReader { isEmpty in
            let action: (() -> Void)? = isEmpty ? nil : { openHistory() }
            CabalMoveReader(cabalID: cabalID) { move in
                CabalPotBand(model: model, move: move, openHistory: action)
            }
        }
    }

    private func openHistory() {
        environment.navigator.open(
            CabalTreasuryHistoryRoute(cabalID: cabalID), in: hostMainTab ?? environment.navigator.selectedTab)
    }
}

struct CabalMoveReader<Content: View>: View {
    let cabalID: String
    @ViewBuilder let content: (CabalHeaderCopy.Move?) -> Content

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.cabalRetry) private var retry
    @Environment(\.cabalModel) private var cabalModel
    @State private var chart: ValueChartModel?

    private var move: CabalHeaderCopy.Move? {
        guard let createdAt = cabalModel?.cabal?.createdAt else { return nil }
        return CabalHeaderCopy.move(chart?.curve, createdAt: createdAt, now: .now)
    }

    var body: some View {
        content(move)
            .task(id: retry.tick) {
                let chart = prepared()
                await withTaskGroup(of: Void.self) { group in
                    group.addTask { await chart.load() }
                    group.addTask { await chart.observe() }
                }
            }
            .onScreenVisibilityChange { chart?.setVisible($0) }
    }

    private func prepared() -> ValueChartModel {
        if let chart { return chart }
        let created = ValueChartModel(
            subjects: [.cabal(id: cabalID)], ranges: [.oneMonth], range: .oneMonth, api: environment.api,
            hints: environment.hints)
        chart = created
        return created
    }
}

struct CabalPotModelHost<Content: View>: View {
    let cabalID: String
    var showsLogos = false
    @ViewBuilder let content: (CabalPotModel?) -> Content

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.cabalRetry) private var retry
    @Environment(\.cabalPotModel) private var shared
    @State private var model: CabalPotModel?

    var body: some View {
        content(shared ?? model)
            .task(id: retry.tick) {
                guard shared == nil else { return }
                let model = self.model ?? makeModel()
                self.model = model
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private func makeModel() -> CabalPotModel {
        CabalPotModel(
            cabalID: cabalID, api: environment.api, hints: environment.hints,
            logoStore: showsLogos ? environment.assetLogos : nil)
    }
}

struct CabalPotBand: View {
    let model: CabalPotModel?
    var move: CabalHeaderCopy.Move?
    var openHistory: (() -> Void)?
    var isPinned = false

    private static let pull = SectionStackMetrics.spacing - MonacoTheme.Space.sm

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        content
            .frame(maxWidth: .infinity, minHeight: isPinned ? 0 : Self.pull + MonacoTheme.Space.xs, alignment: .leading)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, isPinned ? MonacoTheme.Space.xs : 0)
            .padding(.top, isPinned ? 0 : -Self.pull)
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                label
                Text("$0.00").moneyFont(.row).skeletonBar(width: 120)
            }
            .accessibilityIdentifier("cabal-pot-loading")
        case .failed:
            MonacoErrorRow(thing: "the pot", identifier: "cabal-pot-failed", inset: false) {
                Task { await model?.load() }
            }
        case .loaded(let summary):
            loaded(summary)
        }
    }

    @ViewBuilder private func loaded(_ summary: CabalPotSummary) -> some View {
        let figure = line(figure: Text(Self.dimmedCents(summary.potValue)), move: move, chevron: openHistory != nil)
        if let openHistory {
            Button(action: openHistory) {
                figure
                    .contentShape(Rectangle())
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel(Self.spoken(summary.potValue, move: move))
            }
            .buttonStyle(.plain)
            .accessibilityHint(CabalTreasuryCopy.historyHint)
            .accessibilityIdentifier("cabal-pot-value")
        } else {
            figure
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(Self.spoken(summary.potValue, move: move))
                .accessibilityIdentifier("cabal-pot-value")
        }
    }

    private var label: some View {
        Text("Treasury")
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
    }

    private func line(figure: Text, move: CabalHeaderCopy.Move?, chevron: Bool) -> some View {
        let layout =
            dynamicTypeSize >= .xxxLarge
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs))
            : AnyLayout(HStackLayout(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s))
        return HStack(spacing: MonacoTheme.Space.s) {
            layout { parts(figure, move: move) }
            if chevron { MonacoRowChevron() }
        }
    }

    @ViewBuilder private func parts(_ figure: Text, move: CabalHeaderCopy.Move?) -> some View {
        label
        figure
            .moneyFigure(.row)
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(1)
            .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
        if let move {
            Text(move.line)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(Self.color(move.direction))
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("cabal-pot-move")
        }
    }

    static func dimmedCents(_ amount: String) -> AttributedString {
        var text = AttributedString(amount)
        if let dot = amount.lastIndex(of: "."), let range = text.range(of: String(amount[dot...])) {
            text[range].foregroundColor = MonacoTheme.secondaryText
        }
        return text
    }

    static func spoken(_ amount: String, move: CabalHeaderCopy.Move?) -> String {
        guard let move else { return "Treasury, \(amount)" }
        return "Treasury, \(amount), \(PnLSpeech.percent(move.percent)), \(CabalHeaderCopy.moveRange)"
    }

    private static func color(_ direction: PortfolioSummary.Direction) -> Color {
        switch direction {
        case .up: MonacoTheme.profit
        case .down: MonacoTheme.loss
        case .flat: MonacoTheme.secondaryText
        }
    }
}

#if DEBUG
final class CabalPotSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-cabalPotHarness") else { return nil }
        let mode = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "invested"
        return AnyView(CabalPotHarnessScreen(pot: fixture(mode)))
    }

    private static func fixture(_ mode: String) -> Components.Schemas.CabalPot {
        switch mode {
        case "cashOnly": .sampleCashOnly
        case "zero": .sampleZero
        case "outsider": .sampleOutsider
        default: .sampleInvested
        }
    }
}

private struct CabalPotHarnessScreen: View {
    let pot: Components.Schemas.CabalPot

    @State private var model: CabalPotModel?
    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            ScrollView {
                SectionStackLayout(spacing: SectionStackMetrics.spacing) {
                    CabalPotBand(model: model)
                    CabalSliceBand(model: model)
                    CabalTreasuryBody(model: model, cabalID: pot.cabalId, move: nil) { path.append(AnyAppRoute($0)) }
                }
                .padding(.top, MonacoTheme.Space.gutter + SectionStackMetrics.spacing)
            }
            .monacoCanvas()
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
            .task {
                let created = model ?? CabalPotModel.preview(pot)
                model = created
                await created.load()
            }
        }
    }
}
#endif
