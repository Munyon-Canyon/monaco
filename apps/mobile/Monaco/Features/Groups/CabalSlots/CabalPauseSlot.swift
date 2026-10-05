import MonacoCore
import SwiftUI

enum CabalPauseSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalPauseStatus(cabalID: context.cabalID)
    }
}

private struct CabalPauseStatus: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.cabalRetry) private var retry
    @State private var model: CashOutModel?

    var body: some View {
        Group {
            if let pause = model?.preview?.pause {
                CabalPauseRow(pause: pause)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.m)
            } else {
                Color.clear.frame(height: 0)
            }
        }
        .task(id: retry.tick) {
            let model = self.model ?? CashOutModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
            self.model = model
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }
}
