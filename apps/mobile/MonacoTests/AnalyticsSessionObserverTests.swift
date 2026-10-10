import Foundation
import MonacoAnalytics
import Synchronization
import Testing

import struct MonacoCore.SessionProfile

@testable import Monaco

private final class RecordingSink: AnalyticsSink {
    enum Call: Equatable {
        case identify(String, [String: AnalyticsValue])
        case capture(String, [String: AnalyticsValue])
        case screen(String)
        case reset
    }

    private let recorded = Mutex<[Call]>([])

    var calls: [Call] { recorded.withLock { $0 } }

    func identify(userID: String, properties: [String: AnalyticsValue]) {
        recorded.withLock { $0.append(.identify(userID, properties)) }
    }

    func capture(_ name: String, properties: [String: AnalyticsValue]) {
        recorded.withLock { $0.append(.capture(name, properties)) }
    }

    func screen(_ name: String) { recorded.withLock { $0.append(.screen(name)) } }
    func reset() { recorded.withLock { $0.append(.reset) } }
}

private func profile(
    id: String = "01890a5d-ac96-774b-bcce-b302099a8058",
    authState: String = "ONBOARDING_COMPLETED"
) throws -> SessionProfile {
    let json = """
        {"id":"\(id)","handle":"kai","display_name":"Kai Cenat","auth_state":"\(authState)",\
        "account_status":"active","login_provider":"google","member_wallet_address":"wallet-1",\
        "phone_linked":true,"x_username":"kaicenat","created_at":"2026-09-30T12:00:00Z"}
        """
    return try SessionProfile(json: Data(json.utf8))
}

@MainActor
struct AnalyticsSessionObserverTests {
    private let sink = RecordingSink()
    private let analytics: Analytics
    private let observer: AnalyticsSessionObserver

    init() {
        let sink = sink
        let analytics = Analytics(sink: sink)
        self.analytics = analytics
        observer = AnalyticsSessionObserver(
            sessionStore: AppSessionStore(), analytics: analytics, cabalCount: { 2 })
    }

    @Test func identifiesWithOnlyTheFourPersonProperties() async throws {
        await observer.apply(try profile())

        #expect(
            sink.calls == [
                .identify(
                    "01890a5d-ac96-774b-bcce-b302099a8058",
                    [
                        "auth_state": "ONBOARDING_COMPLETED", "login_provider": "google", "cabal_count": 2,
                        "created_at": "2026-09-30T12:00:00Z",
                    ])
            ])
    }

    @Test func personPropertiesCarryNoContactDetails() async throws {
        await observer.apply(try profile())

        guard case .identify(_, let properties)? = sink.calls.first else {
            Issue.record("no identify")
            return
        }
        #expect(Set(properties.keys) == ["auth_state", "login_provider", "cabal_count", "created_at"])
        let rendered = "\(properties)"
        for secret in ["wallet-1", "kaicenat", "Kai Cenat", "kai"] {
            #expect(!rendered.contains(secret))
        }
    }

    @Test func identifiesOncePerUser() async throws {
        await observer.apply(try profile())
        await observer.apply(try profile())

        #expect(sink.calls.count == 1)
    }

    @Test func identifiesAgainWhenTheAuthStateMoves() async throws {
        await observer.apply(try profile(authState: "CREATED"))
        await observer.apply(try profile(authState: "ONBOARDING_COMPLETED"))

        let identifies = sink.calls.filter { if case .identify = $0 { true } else { false } }
        #expect(identifies.count == 2)
    }

    @Test func signOutResetsOnce() async throws {
        await observer.apply(try profile())
        await observer.apply(nil)
        await observer.apply(nil)

        #expect(sink.calls.filter { $0 == .reset }.count == 1)
    }

    @Test func aFreshLoginCompletesTheOnboardingAttemptWithOneFlowID() async throws {
        analytics.loginStarted()
        await observer.apply(try profile(authState: "CREATED"))

        let flowIDs = sink.calls.compactMap { call -> AnalyticsValue? in
            guard case .capture(_, let properties) = call else { return nil }
            return properties["flow_id"]
        }
        let names = sink.calls.compactMap { call -> String? in
            guard case .capture(let name, _) = call else { return nil }
            return name
        }
        #expect(names == ["onboarding_login_started", "onboarding_login_completed"])
        #expect(flowIDs.count == 2 && flowIDs[0] == flowIDs[1])
    }

    @Test func aRestoredSessionDoesNotCountAsALogin() async throws {
        await observer.apply(try profile())

        #expect(!sink.calls.contains { if case .capture = $0 { true } else { false } })
    }

    @Test func aMissingCabalCountIsLeftOut() async throws {
        let observer = AnalyticsSessionObserver(
            sessionStore: AppSessionStore(), analytics: analytics, cabalCount: { nil })
        await observer.apply(try profile())

        guard case .identify(_, let properties)? = sink.calls.first else {
            Issue.record("no identify")
            return
        }
        #expect(properties["cabal_count"] == nil)
    }
}

@MainActor
struct AppAnalyticsSinkChoiceTests {
    @Test func anEmptyKeyLogsOnly() {
        #expect(AppAnalytics.makeSink(apiKey: "", isTest: false) is LoggingAnalyticsSink)
    }

    @Test func testsNeverReachPostHog() {
        #expect(AppAnalytics.makeSink(apiKey: "phc_test_placeholder", isTest: true) is LoggingAnalyticsSink)
    }

    @Test func referralEventsKeepTheirNamesAndShareAFlowID() {
        let sink = RecordingSink()
        AppAnalytics.current = Analytics(sink: sink)
        AppAnalytics.capture(.invitePasteShown)
        AppAnalytics.capture(.invitePasted)

        let names = sink.calls.compactMap { call -> String? in
            guard case .capture(let name, _) = call else { return nil }
            return name
        }
        #expect(names == ["invite_paste_shown", "invite_pasted"])
    }
}
