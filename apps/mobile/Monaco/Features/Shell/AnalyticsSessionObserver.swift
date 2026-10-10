import Foundation
import MonacoAnalytics
import MonacoCore
import Observation

@MainActor
final class AnalyticsSessionObserver {
    private let sessionStore: AppSessionStore
    private let analytics: Analytics
    private let cabalCount: @MainActor () async -> Int?
    private var identified: (userID: String, authState: SessionProfile.AuthState)?

    init(
        sessionStore: AppSessionStore,
        analytics: Analytics,
        cabalCount: @escaping @MainActor () async -> Int?
    ) {
        self.sessionStore = sessionStore
        self.analytics = analytics
        self.cabalCount = cabalCount
    }

    func start() {
        track()
    }

    func apply(_ profile: SessionProfile?) async {
        guard let profile else {
            guard identified != nil else { return }
            identified = nil
            analytics.reset()
            return
        }
        let current = (userID: profile.userID, authState: profile.authState)
        guard identified?.userID != current.userID || identified?.authState != current.authState else { return }
        let isNewUser = identified?.userID != current.userID
        identified = current
        let count = isNewUser ? await cabalCount() : nil
        guard identified?.userID == current.userID else { return }
        analytics.identify(userID: profile.userID, properties: Self.properties(for: profile, cabalCount: count))
        if isNewUser, analytics.hasAttempt(for: .onboarding) {
            analytics.step(
                .onboarding(.loginCompleted),
                properties: [Self.loginProvider: .string(Self.name(of: profile.loginProvider))]
            )
        }
    }

    static func properties(for profile: SessionProfile, cabalCount: Int?) -> [String: AnalyticsValue] {
        var properties: [String: AnalyticsValue] = [
            "auth_state": .string(name(of: profile.authState)),
            Self.loginProvider: .string(name(of: profile.loginProvider)),
            "created_at": .string(profile.createdAt.formatted(.iso8601)),
        ]
        if let cabalCount { properties["cabal_count"] = .int(cabalCount) }
        return properties
    }

    private static let loginProvider = "login_provider"

    private func track() {
        withObservationTracking {
            _ = sessionStore.profile
        } onChange: { [weak self] in
            Task { @MainActor in
                guard let self else { return }
                self.track()
                await self.apply(self.sessionStore.profile)
            }
        }
    }

    private static func name(of state: SessionProfile.AuthState) -> String {
        switch state {
        case .created: "CREATED"
        case .awaitingPhone: "AWAITING_PHONE"
        case .awaitingSocials: "AWAITING_SOCIALS"
        case .onboardingCompleted: "ONBOARDING_COMPLETED"
        case .unknown(let raw): raw
        }
    }

    private static func name(of provider: SessionProfile.LoginProvider) -> String {
        switch provider {
        case .sms: "sms"
        case .email: "email"
        case .apple: "apple"
        case .google: "google"
        case .unknown(let raw): raw
        }
    }
}
