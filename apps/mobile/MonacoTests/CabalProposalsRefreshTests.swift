import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI
import Synchronization
import Testing
import UIKit

@testable import Monaco

@MainActor
struct CabalProposalsRefreshTests {
    @Test(.timeLimit(.minutes(1)))
    func aProposalCreatedWhileTheCabalIsOpenIsFetched() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, #"{"proposals":[],"next_cursor":null}"#),
            .json(.ok, #"{"proposals":[],"next_cursor":null}"#),
        ])
        let hints = EmittingHintSource()
        let model = ProposalListModel(
            cabalID: "cabal-1", filter: .open,
            repository: ProposalsRepository(
                api: APIClient(
                    serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)),
            hints: hints)
        let window = try ProposalTestWindow.hosting(CabalProposals(cabalID: "cabal-1", makeModel: { _, _ in model }))
        defer { window.isHidden = true }

        await ProposalTestWindow.until { model.pager.phase == .exhausted && hints.subscribers > 0 }
        model.setVisible(true)
        hints.send(.changed(.cabal("cabal-1"), what: "proposal_created", id: "1"))
        await ProposalTestWindow.until { await listRequests(transport) == 2 }

        #expect(await listRequests(transport) == 2)
    }

    @Test(.timeLimit(.minutes(1)))
    func aVoteThatClosesAProposalMovesItFromOpenToExecuted() async throws {
        let open = Self.proposalJSON(status: "open")
        let executed = Self.proposalJSON(status: "executed")
        let transport = StubTransport(scripted: [
            .json(.ok, #"{"proposals":[\#(open)],"next_cursor":null}"#),
            .json(.ok, #"{"proposals":[],"next_cursor":null}"#),
            .json(.ok, #"{"proposals":[],"next_cursor":null}"#),
            .json(.ok, #"{"proposals":[\#(executed)],"next_cursor":null}"#),
        ])
        let segments = ProposalSegmentsModel(
            cabalID: "cabal-1",
            repository: ProposalsRepository(
                api: APIClient(
                    serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)),
            hints: EmittingHintSource())
        await segments.model(for: .open).load()
        await segments.model(for: .executed).load()
        #expect(segments.model(for: .open).pager.items.map(\.id) == ["p"])

        await segments.refreshLoaded()

        #expect(segments.model(for: .open).pager.items.isEmpty)
        #expect(segments.model(for: .executed).pager.items.map(\.id) == ["p"])
    }

    private static func proposalJSON(status: String) -> String {
        #"{"id":"p","cabal_id":"cabal-1","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"\#(status)","status_reason":null,"status_message":null,"expires_at":"2099-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":1,"no":0,"voters":1,"needed":1},"my_ballot":"yes","can_vote":true}"#
    }

    private func listRequests(_ transport: StubTransport) async -> Int {
        await transport.sent.filter { ($0.path ?? "").hasPrefix("/v1/cabals/cabal-1/proposals") }.count
    }
}

nonisolated final class EmittingHintSource: HintConnecting, Sendable {
    private let continuations = Mutex<[(HintFilter, AsyncStream<Hint>.Continuation)]>([])

    var subscribers: Int { continuations.withLock { $0.count } }

    func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
        let (stream, continuation) = AsyncStream.makeStream(of: Hint.self)
        continuations.withLock { $0.append((filter, continuation)) }
        return stream
    }

    func send(_ hint: Hint) {
        for (filter, continuation) in continuations.withLock({ $0 }) where filter.matches(hint) {
            continuation.yield(hint)
        }
    }

    func start() async {}
    func stop() async {}
}

@MainActor
enum ProposalTestWindow {
    static func hosting(_ view: some View) throws -> UIWindow {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let environment = AppEnvironment(
            auth: auth, hints: FakeHintSource(), isAuthenticated: { true }, endAuthSession: {})
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack { view }
                .environment(environment)
                .environment(ToastCenter())
        )
        window.makeKeyAndVisible()
        return window
    }

    static func until(_ predicate: () async -> Bool) async {
        for _ in 0..<5_000 {
            if await predicate() { return }
            await Task.yield()
        }
    }
}
