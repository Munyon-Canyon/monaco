import MonacoAPI
import MonacoCore
import SwiftUI

@MainActor
enum ShareCardLauncher {
    static let failure = "Couldn\u{2019}t open sharing. Try again."
    static let meKey = "me"

    static func shareCabal(id: String, api: APIClient, toasts: ToastCenter?, busy: Binding<String?>) async {
        await prepare(key: id, api: api, toasts: toasts, busy: busy) { model in
            guard
                let cabal = try? await api.read({ try await $0.getCabal(path: .init(id: id)).ok.body.json })
            else { return }
            await model.loadCabal(cabalID: id, cabal: cabal)
        }
    }

    static func shareCabal(
        _ cabal: Components.Schemas.Cabal, api: APIClient, toasts: ToastCenter?, busy: Binding<String?>
    ) async {
        await prepare(key: cabal.id, api: api, toasts: toasts, busy: busy) { model in
            await model.loadCabal(cabalID: cabal.id, cabal: cabal)
        }
    }

    static func shareMe(profile: SessionProfile, api: APIClient, toasts: ToastCenter?, busy: Binding<String?>) async {
        await prepare(key: meKey, api: api, toasts: toasts, busy: busy) { model in
            await model.loadMe(profile: profile)
        }
    }

    private static func prepare(
        key: String, api: APIClient, toasts: ToastCenter?, busy: Binding<String?>,
        load: (ShareCardModel) async -> Void
    ) async {
        guard busy.wrappedValue == nil else { return }
        busy.wrappedValue = key
        defer { busy.wrappedValue = nil }
        let model = ShareCardModel(api: api)
        await load(model)
        guard let content = model.content else {
            toasts?.current = MonacoToast(message: failure)
            return
        }
        let face = await ShareCardRenderer.face(for: content.faceURL)
        SystemShare.present(.card(content, image: ShareCardRenderer.render(content, face: face)))
    }
}
