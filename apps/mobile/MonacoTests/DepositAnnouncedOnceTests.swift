import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import Observation
import OpenAPIRuntime
import SwiftUI
import Synchronization
import Testing
import UIKit
import UserNotifications

@testable import Monaco

@MainActor
struct DepositAnnouncedOnceTests {
    @Test(.timeLimit(.minutes(1)))
    func oneDepositToastsOnceWhileHomeAndProfileBothShowTheBalance() async throws {
        let app = try Harness(available: 12_000_000)
        defer { app.close() }
        await app.settledShowing(12_000_000)

        await app.server.setAvailable(37_000_000)
        await app.resendDepositHintUntilShowing(37_000_000)
        await app.turns(2_000)

        #expect(app.toastCount.shown == 1)
        #expect(app.toasts.current?.message == "Deposit received: $25.00")
    }

    @Test(.timeLimit(.minutes(1)))
    func aFailedReloadToastsOnceWhateverNumberOfScreensShowTheBalance() async throws {
        let app = try Harness(available: 12_000_000)
        defer { app.close() }
        await app.settledShowing(12_000_000)

        await app.server.setFailing(true)
        await app.environment.balance.load()
        await app.turns(2_000)

        #expect(app.toastCount.shown == 1)
        #expect(
            app.toasts.current?.message == BalanceSource.message(for: .transport(URLError(.notConnectedToInternet))))
        #expect(app.environment.balance.balance?.availableMicros == 12_000_000)
    }

    @Test func signingOutForgetsTheBalanceTheNextAccountWouldInherit() async {
        let server = BalanceServer(available: 12_000_000)
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(),
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: server),
            hints: ScriptedHints(), isAuthenticated: { true }, endAuthSession: {})
        await environment.balance.load()
        #expect(environment.balance.balance?.availableMicros == 12_000_000)

        await environment.signOut()

        #expect(environment.balance.state == .idle)
    }

    @Test func aDepositPushIsLeftToTheToastWhileTheAppIsOpen() {
        #expect(PushCenterDelegate.presentation(for: ["kind": "deposit_credited"]) == [])
    }

    @Test func everyOtherPushStillShowsAsABannerWhileTheAppIsOpen() {
        #expect(PushCenterDelegate.presentation(for: ["kind": "new_follower"]) == [.banner, .list, .sound])
        #expect(PushCenterDelegate.presentation(for: [:]) == [.banner, .list, .sound])
    }
}

@MainActor
private final class Harness {
    let server: BalanceServer
    let hints = ScriptedHints()
    let toasts = ToastCenter()
    let environment: AppEnvironment
    let toastCount: ToastWriteCount
    private let window: UIWindow

    init(available: Int64) throws {
        server = BalanceServer(available: available)
        environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(),
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: server),
            hints: hints, isAuthenticated: { true }, endAuthSession: {})
        toastCount = ToastWriteCount(toasts)
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene")
        window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack {
                VStack {
                    HomeBalanceRowSection()
                    HomeBalanceRowSection(identifierPrefix: "profile", balanceIdentifier: "profile-balance-value")
                }
            }
            .modifier(AccountBalanceHost())
            .environment(environment)
            .environment(toasts)
        )
        window.makeKeyAndVisible()
    }

    func close() {
        window.isHidden = true
    }

    func settledShowing(_ micros: Int64) async {
        await until { self.environment.balance.balance?.availableMicros == micros }
        await turns(200)
    }

    func resendDepositHintUntilShowing(_ micros: Int64) async {
        await until {
            self.hints.send(.changed(.user(UUID().uuidString), what: BalanceSource.refreshingHint, id: "1"))
            return self.environment.balance.balance?.availableMicros == micros
        }
    }

    func turns(_ count: Int) async {
        for _ in 0..<count { await Self.nextRunLoopTurn() }
    }

    private func until(_ condition: () async -> Bool) async {
        for _ in 0..<10_000 {
            if await condition() { return }
            await Self.nextRunLoopTurn()
        }
    }

    private static func nextRunLoopTurn() async {
        await withCheckedContinuation { continuation in
            RunLoop.main.perform { continuation.resume() }
        }
    }
}

@MainActor
private final class ToastWriteCount {
    private(set) var shown = 0

    init(_ center: ToastCenter) {
        watch(center)
    }

    private func watch(_ center: ToastCenter) {
        withObservationTracking {
            _ = center.current
        } onChange: { [self] in
            MainActor.assumeIsolated {
                shown += 1
                watch(center)
            }
        }
    }
}

private actor BalanceServer: ClientTransport {
    private var available: Int64
    private var failing = false

    init(available: Int64) {
        self.available = available
    }

    func setAvailable(_ micros: Int64) {
        available = micros
    }

    func setFailing(_ failing: Bool) {
        self.failing = failing
    }

    func send(_ request: HTTPRequest, body: HTTPBody?, baseURL: URL, operationID: String) async throws
        -> (HTTPResponse, HTTPBody?)
    {
        guard request.path == "/v1/me/balance", !failing else { throw URLError(.notConnectedToInternet) }
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        let json =
            #"{"available_micros":"\#(available)","on_chain_micros":"\#(available)","in_flight_micros":"0","#
            + #""deposit_address":"wallet-1","as_of":"2025-10-04T12:00:00Z"}"#
        return (response, HTTPBody(Data(json.utf8)))
    }
}

private nonisolated final class ScriptedHints: HintConnecting, Sendable {
    private let listeners = Mutex<[UUID: (filter: HintFilter, continuation: AsyncStream<Hint>.Continuation)]>([:])

    func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
        let (stream, continuation) = AsyncStream.makeStream(of: Hint.self)
        let id = UUID()
        listeners.withLock { $0[id] = (filter, continuation) }
        continuation.onTermination = { [self] _ in listeners.withLock { $0[id] = nil } }
        return stream
    }

    func send(_ hint: Hint) {
        let matching = listeners.withLock { $0.values.filter { $0.filter.matches(hint) } }
        for listener in matching { listener.continuation.yield(hint) }
    }

    func start() async {}
    func stop() async {}
}
