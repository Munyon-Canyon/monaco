import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalPotSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalPotModelHost(cabalID: context.cabalID) { model in
            CabalPotBand(model: model)
        }
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

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, -(SectionStackMetrics.spacing - MonacoTheme.Space.gutter))
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            Text("$0.00").moneyFont(.hero).skeletonBar(width: 180)
                .accessibilityIdentifier("cabal-pot-loading")
        case .failed:
            MonacoErrorRow(thing: "the pot", identifier: "cabal-pot-failed", inset: false) {
                Task { await model?.load() }
            }
        case .loaded(let summary):
            Text(summary.potValue)
                .moneyFont(.hero)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .accessibilityLabel("In the pot, \(summary.potValue)")
                .accessibilityIdentifier("cabal-pot-value")
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
                    CabalHoldingsSection(model: model, cabalID: pot.cabalId) { path.append(AnyAppRoute($0)) }
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
