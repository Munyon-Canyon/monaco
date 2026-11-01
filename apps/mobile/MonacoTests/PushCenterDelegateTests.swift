import MonacoCore
import Testing
import UserNotifications

@testable import Monaco

@MainActor
struct PushCenterDelegateTests {
    private let proposal = "0192e8a1-0003-7000-8000-000000000003"

    @Test func depositCreditedStaysSilentInTheForeground() {
        #expect(PushCenterDelegate.presentation(for: ["kind": "deposit_credited"]) == [])
    }

    @Test func voteKindsShowBannerListAndSound() {
        let info: [AnyHashable: Any] = ["kind": "vote_reminder", "proposal_id": proposal]
        #expect(PushCenterDelegate.presentation(for: info) == [.banner, .list, .sound])
    }

    @Test func aShownProposalOnlyJoinsTheList() {
        let info: [AnyHashable: Any] = ["kind": "vote_reminder", "proposal_id": proposal]
        #expect(PushCenterDelegate.presentation(for: info, shownProposalID: proposal) == [.list])
        #expect(PushCenterDelegate.presentation(for: info, shownProposalID: "other") == [.banner, .list, .sound])
    }

    @Test func aShownChatOnlyJoinsTheList() {
        let cabal = "0192e8a1-0002-7000-8000-000000000002"
        let info: [AnyHashable: Any] = ["kind": "chat_mention", "cabal_id": cabal, "message_id": proposal]
        let other: [AnyHashable: Any] = ["kind": "chat_mention", "cabal_id": proposal, "message_id": proposal]
        let access: [AnyHashable: Any] = ["kind": "cabal_access_requested", "cabal_id": cabal]
        #expect(PushCenterDelegate.presentation(for: info, shownChatCabalID: cabal) == [.list])
        #expect(PushCenterDelegate.presentation(for: other, shownChatCabalID: cabal) == [.banner, .list, .sound])
        #expect(PushCenterDelegate.presentation(for: access, shownChatCabalID: cabal) == [.banner, .list, .sound])
        #expect(PushCenterDelegate.presentation(for: info) == [.banner, .list, .sound])
    }

    @Test func categoriesMapToNotificationCategories() {
        let mapped = PushCategory.all.map(\.notificationCategory)
        let categories = Dictionary(uniqueKeysWithValues: mapped.map { ($0.identifier, $0) })
        #expect(Set(categories.keys) == ["VOTE", "VOTE_DECIDING", "LEDGER"])
        #expect(categories["VOTE"]?.actions.map(\.identifier) == ["VOTE_YES", "VOTE_NO", "READ_THESIS"])
        #expect(categories["VOTE"]?.actions.first?.options == [.foreground, .authenticationRequired])
    }
}
