import MonacoCore
import MonacoReferrals
import SwiftUI

nonisolated struct InviteLandingSignIn: Hashable, Sendable {
    let run: @MainActor @Sendable () -> Void

    static func == (_: Self, _: Self) -> Bool { true }
    func hash(into _: inout Hasher) {}
}

nonisolated struct InviteLandingRoute: AppRoute {
    let code: ReferralCode
    let cabalID: String
    var onSignIn = InviteLandingSignIn(run: {})

    init(code: ReferralCode, cabalID: String, onSignIn: @escaping @MainActor @Sendable () -> Void = {}) {
        self.code = code
        self.cabalID = cabalID
        self.onSignIn = InviteLandingSignIn(run: onSignIn)
    }

    @MainActor func destination() -> some View {
        InviteLandingDestination(route: self)
    }
}

struct InviteLandingDestination: View {
    let route: InviteLandingRoute

    @Environment(AppEnvironment.self) private var environment
    @State private var model: InviteLandingModel?

    var body: some View {
        Group {
            if let model {
                InviteLandingScreen(
                    model: model, shareURL: ReferralLink.url(for: route.code, cabalID: route.cabalID),
                    onRedirect: redirect)
            } else {
                MonacoTheme.canvas.ignoresSafeArea()
            }
        }
        .task {
            let model =
                self.model
                ?? InviteLandingModel(
                    code: route.code.value, cabalID: route.cabalID, api: environment.api,
                    hints: environment.viewer == nil ? nil : environment.hints, viewerID: environment.viewer?.userID)
            self.model = model
            await model.run()
        }
    }

    private func redirect(_ target: InviteLandingModel.Redirect) {
        let navigator = environment.navigator
        let tab = navigator.selectedTab
        let own = AnyAppRoute(route)
        switch target {
        case .cabal(let id):
            navigator.cabalsPath.removeAll { $0 == own }
            navigator.open(CabalRoute(id: id), in: .cabals)
        case .referrer(let value):
            guard let code = ReferralCode(value) else { return }
            navigator.cabalsPath.removeAll { $0 == own }
            navigator.open(ReferralRoute(code: code), in: tab)
        case .signIn:
            route.onSignIn.run()
        }
    }
}
