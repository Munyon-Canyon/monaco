import Foundation
import MonacoAPI
import Testing

import struct MonacoCore.SettlementTicket

@testable import Monaco

@MainActor
struct PendingMoneyEnvironmentTests {
    private func environment(_ journal: PendingIntentJournal, userID: String) -> AppEnvironment {
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: StubTransport(.hang))
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(), api: api, hints: FakeHintSource(),
            pendingIntents: journal, isAuthenticated: { true }, endAuthSession: {}
        )
        environment.viewer = Viewer(userID: userID, handle: nil)
        return environment
    }

    @Test func signingOutCancelsTheWatchersAndKeepsTheIntent() async {
        let journal = PendingIntentJournal()
        let environment = environment(journal, userID: "user-a")
        environment.settlements.bind(accountID: "user-a")
        environment.settlements.watch(SettlementTicket(kind: .withdraw, resourceID: "withdrawal-1"))
        #expect(environment.settlements.watchedCount == 1)

        await environment.signOut()

        #expect(environment.settlements.watchedCount == 0)
        #expect(journal.intents(accountID: "user-a").count == 1)
    }

    @Test func aSubmissionOnlySeesTheSignedInAccountsPendingKey() {
        let journal = PendingIntentJournal()
        let first = environment(journal, userID: "user-a").moneySubmission(slot: "withdraw")
        _ = first.key(fingerprint: Data("withdraw {}".utf8))

        #expect(first.hasPendingKey)
        #expect(environment(journal, userID: "user-a").moneySubmission(slot: "withdraw").hasPendingKey)
        #expect(!environment(journal, userID: "user-b").moneySubmission(slot: "withdraw").hasPendingKey)
    }
}
