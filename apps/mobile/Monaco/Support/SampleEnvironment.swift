#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

extension AppEnvironment {
    static func sample(auth: PrivyAuthService, suite: String) -> AppEnvironment {
        let tokens = SessionTokens(privyToken: { "sample-token" }, refresh: { _ in nil })
        UserDefaults(suiteName: suite)?.removePersistentDomain(forName: suite)
        return AppEnvironment(
            auth: auth, tokens: tokens,
            api: APIClient(serverURL: URL(string: "https://sample.monaco.test") ?? URL(filePath: "/"), tokens: tokens),
            hints: SampleSilentHints(), gifs: SampleGifProvider(),
            defaults: UserDefaults(suiteName: suite) ?? .standard, isAuthenticated: { true }, endAuthSession: {})
    }
}
#endif
