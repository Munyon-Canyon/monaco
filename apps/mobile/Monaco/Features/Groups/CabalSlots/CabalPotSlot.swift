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
    @ViewBuilder let content: (CabalPotModel?) -> Content

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.cabalRetry) private var retry
    @State private var model: CabalPotModel?

    var body: some View {
        content(model)
            .task(id: retry.tick) {
                let model = self.model ?? makeModel()
                self.model = model
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private func makeModel() -> CabalPotModel {
        CabalPotModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
    }
}

struct CabalPotBand: View {
    let model: CabalPotModel?

    var body: some View {
        CabalInkBand {
            Text("In the pot")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.onHero)
                .accessibilityAddTraits(.isHeader)
            content
        }
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            SkeletonBlock(width: 180, height: 44)
            SkeletonBlock(width: 120, height: 24, radius: 12)
        case .failed:
            MonacoErrorRow(thing: "the pot", identifier: "cabal-pot-failed", onHero: true) {
                Task { await model?.load() }
            }
        case .loaded(let summary):
            Text(summary.potValue)
                .moneyFont(.hero)
                .foregroundStyle(MonacoTheme.onHero)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .accessibilityIdentifier("cabal-pot-value")
            AllTimeChip(allTime: summary.allTime)
        }
    }
}

private struct AllTimeChip: View {
    let allTime: String

    private var tone: PnLTone { PnLTone(dollarPnl: allTime) }

    private var label: String {
        let magnitude = allTime.drop(while: { $0 == "+" || $0 == "\u{2212}" })
        return switch tone {
        case .profit: "▲ \(magnitude) · all time"
        case .loss: "▼ \(magnitude) · all time"
        case .flat: "\(magnitude) · all time"
        }
    }

    var body: some View {
        Text(label)
            .moneyFont(.caption, weight: .semibold)
            .foregroundStyle(tone.inkCardColor)
            .lineLimit(1)
            .padding(.horizontal, MonacoTheme.Space.s)
            .padding(.vertical, MonacoTheme.Space.xs)
            .background(Capsule().fill(tone.inkCardWash))
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("All-time gain")
            .accessibilityValue(allTime)
            .accessibilityIdentifier("cabal-pot-all-time")
    }
}

struct CabalInkBand<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            content
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background {
            MonacoTheme.heroInk.padding(.top, -MonacoTheme.Space.gutter)
        }
    }
}

struct CabalInkCaption: View {
    let text: String
    let id: String

    init(_ text: String, id: String) {
        self.text = text
        self.id = id
    }

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.onHeroMuted)
            .accessibilityIdentifier(id)
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
                VStack(spacing: 0) {
                    CabalPotBand(model: model)
                    CabalSliceBand(model: model)
                    CabalHoldingsSection(model: model, cabalID: pot.cabalId) { path.append(AnyAppRoute($0)) }
                        .padding(.top, MonacoTheme.Space.l)
                }
                .padding(.top, MonacoTheme.Space.gutter)
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
