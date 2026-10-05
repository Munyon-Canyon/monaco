import MonacoCore
import SwiftUI

enum CabalSliceSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalSlice(cabalID: context.cabalID)
    }
}

private struct CabalSlice: View {
    let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.cabalRetry) private var retry
    @State private var model: CabalActionsModel?

    var body: some View {
        Group {
            if case .member = model?.actions {
                CabalInkBand {
                    Rectangle()
                        .fill(MonacoTheme.onHeroHairline)
                        .frame(height: 1)
                        .padding(.bottom, MonacoTheme.Space.s)
                        .accessibilityHidden(true)
                    Text("Your slice")
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.onHero)
                        .accessibilityAddTraits(.isHeader)
                    CabalInkCaption("Your slice shows up here soon.", id: "cabal-slice-coming")
                }
            } else {
                Color.clear.frame(height: 0)
            }
        }
        .task(id: retry.tick) {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
    }

    private func preparedModel() -> CabalActionsModel {
        if let model { return model }
        let created = CabalActionsModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}
