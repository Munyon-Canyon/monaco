import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct GroupCashRoute: AppRoute {
    let cabalID: String
    let cabalName: String

    @MainActor func destination() -> some View {
        CabalPotModelHost(cabalID: cabalID) { model in
            GroupCashScreen(model: model, cabalID: cabalID, cabalName: cabalName)
        }
        .analyticsScreen("cabal_cash")
    }
}

private struct GroupCashScreen: View {
    let model: CabalPotModel?
    let cabalID: String
    let cabalName: String

    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        GroupCashView(
            phase: phase, cabalID: cabalID, cabalName: cabalName,
            reload: { await model?.load() }
        )
        .onChange(of: model?.toast) { _, message in
            guard let model, let message else { return }
            toasts.current = MonacoToast(message: message)
            model.dismissToast()
        }
    }

    private var phase: GroupCashView.Phase {
        switch model?.state ?? .loading {
        case .idle, .loading: .loading
        case .failed(_): .failed
        case .loaded(let summary): .loaded(GroupCashSummary(summary))
        }
    }
}
