import Foundation
import Observation

@Observable
@MainActor
public final class PushPrePrompt {
    private static let shownKey = "pushPrePromptShown"

    public var isPresented = false

    private let authorization: any NotificationAuthorizing
    private let defaults: UserDefaults
    private let register: @MainActor () -> Void

    public init(
        authorization: any NotificationAuthorizing,
        defaults: UserDefaults,
        register: @escaping @MainActor () -> Void
    ) {
        self.authorization = authorization
        self.defaults = defaults
        self.register = register
    }

    public func noteCabalJoined() async {
        let status = await authorization.status()
        let alreadyShown = defaults.bool(forKey: Self.shownKey)
        guard PushPrePromptPolicy.shouldShow(status: status, alreadyShown: alreadyShown) else { return }
        isPresented = true
    }

    public func turnOn() async {
        markShown()
        let granted = await authorization.request()
        isPresented = false
        if granted { register() }
    }

    public func notNow() {
        markShown()
        isPresented = false
    }

    private func markShown() {
        defaults.set(true, forKey: Self.shownKey)
    }
}
