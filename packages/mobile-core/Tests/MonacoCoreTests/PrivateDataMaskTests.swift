import Foundation
import XCTest

final class PrivateDataMaskTests: XCTestCase {
    static let requiredMasks: [String: Int] = [
        "apps/mobile/Monaco/Design/CabalCover.swift": 1,
        "apps/mobile/Monaco/Design/CabalCoverCard.swift": 3,
        "apps/mobile/Monaco/Design/MonacoAmountEntry.swift": 4,
        "apps/mobile/Monaco/Design/MonacoAvatar.swift": 1,
        "apps/mobile/Monaco/Design/MonacoBoardRow.swift": 1,
        "apps/mobile/Monaco/Design/MonacoList.swift": 1,
        "apps/mobile/Monaco/Design/MonacoMoney.swift": 5,
        "apps/mobile/Monaco/Design/MonacoTextField.swift": 2,
        "apps/mobile/Monaco/Design/MonacoWalletAddressText.swift": 1,
        "apps/mobile/Monaco/Design/OutcomeView.swift": 1,
        "apps/mobile/Monaco/Design/ReceiptLine.swift": 1,
        "apps/mobile/Monaco/Features/Auth/OTPCodeField.swift": 1,
        "apps/mobile/Monaco/Features/Auth/OTPLoginForm.swift": 1,
        "apps/mobile/Monaco/Features/Comments/CommentComposer.swift": 1,
        "apps/mobile/Monaco/Features/Comments/CommentThreadView.swift": 2,
        "apps/mobile/Monaco/Features/Deposit/DepositView.swift": 1,
        "apps/mobile/Monaco/Features/Deposit/FundCabalScreen.swift": 2,
        "apps/mobile/Monaco/Features/Deposit/GroupCashView.swift": 1,
        "apps/mobile/Monaco/Features/Feed/FeedEventParts.swift": 4,
        "apps/mobile/Monaco/Features/Feed/FeedItemDetailView.swift": 3,
        "apps/mobile/Monaco/Features/Friends/FriendsPersonRow.swift": 1,
        "apps/mobile/Monaco/Features/Groups/CabalHoldingParts.swift": 1,
        "apps/mobile/Monaco/Features/Groups/CabalSlots/CabalActivitySlot.swift": 1,
        "apps/mobile/Monaco/Features/Groups/CabalSlots/CabalHoldingsSlot.swift": 3,
        "apps/mobile/Monaco/Features/Groups/CabalSlots/CabalMemberBoardSlot.swift": 1,
        "apps/mobile/Monaco/Features/Groups/CabalSlots/CabalPotSlot.swift": 1,
        "apps/mobile/Monaco/Features/Groups/CabalSlots/CabalSliceSlot.swift": 1,
        "apps/mobile/Monaco/Features/Groups/CabalSlots/CabalValueChartSlot.swift": 1,
        "apps/mobile/Monaco/Features/Groups/CabalTransactionView.swift": 1,
        "apps/mobile/Monaco/Features/Groups/ChatMentionPicker.swift": 1,
        "apps/mobile/Monaco/Features/Groups/ChatMessageText.swift": 1,
        "apps/mobile/Monaco/Features/Groups/ChatThreadView.swift": 1,
        "apps/mobile/Monaco/Features/Groups/GroupChatParts.swift": 1,
        "apps/mobile/Monaco/Features/Groups/GroupChatRowView.swift": 1,
        "apps/mobile/Monaco/Features/Groups/InviteMemberView.swift": 1,
        "apps/mobile/Monaco/Features/Groups/MembersView.swift": 3,
        "apps/mobile/Monaco/Features/Home/BalancesView.swift": 4,
        "apps/mobile/Monaco/Features/Home/HomePortfolioSlot.swift": 2,
        "apps/mobile/Monaco/Features/Notifications/ActivityView.swift": 2,
        "apps/mobile/Monaco/Features/Onboarding/HandleStepView.swift": 1,
        "apps/mobile/Monaco/Features/Onboarding/PhoneStepView.swift": 2,
        "apps/mobile/Monaco/Features/Profile/DisplayNameField.swift": 1,
        "apps/mobile/Monaco/Features/Profile/IdentityCard.swift": 5,
        "apps/mobile/Monaco/Features/Profile/InviteView.swift": 2,
        "apps/mobile/Monaco/Features/Proposals/ProposalCard.swift": 2,
        "apps/mobile/Monaco/Features/Proposals/ProposalHero.swift": 1,
        "apps/mobile/Monaco/Features/Proposals/ProposalListRow.swift": 1,
        "apps/mobile/Monaco/Features/Proposals/ProposePotTotalRow.swift": 1,
        "apps/mobile/Monaco/Features/Proposals/ProposeReviewScreen.swift": 1,
        "apps/mobile/Monaco/Features/Referrals/ReferralCodeSheet.swift": 1,
        "apps/mobile/Monaco/Features/Settings/AccountActivityView.swift": 1,
        "apps/mobile/Monaco/Features/Settings/BlockedPeopleRoute.swift": 1,
        "apps/mobile/Monaco/Features/Settings/DeleteAccountView.swift": 1,
        "apps/mobile/Monaco/Features/Settings/ReceiptView.swift": 1,
        "apps/mobile/Monaco/Features/Settings/WithdrawScreen.swift": 1,
    ]

    static let pausedFiles = [
        "apps/mobile/Monaco/Features/Auth/OTPLoginForm.swift",
        "apps/mobile/Monaco/Features/Onboarding/PhoneStepView.swift",
    ]

    private static let maskPattern = #"\.monacoPrivate(?:Figure)?\(|\.moneyFigure\(|isPrivate: true"#

    func testEveryKnownMoneyAndIdentityViewKeepsItsMask() throws {
        let regex = try NSRegularExpression(pattern: Self.maskPattern)
        var missing: [String] = []
        for (path, minimum) in Self.requiredMasks.sorted(by: { $0.key < $1.key }) {
            let source = try RepoTree.read(path)
            let found = regex.numberOfMatches(in: source as String, range: NSRange(location: 0, length: source.length))
            if found < minimum { missing.append("\(path) has \(found) of \(minimum) masks") }
        }
        XCTAssertTrue(missing.isEmpty, "a money or identity view lost its mask modifier: \(missing)")
    }

    func testTheSignInCodeScreensPauseSessionReplay() throws {
        for path in Self.pausedFiles {
            let source = try RepoTree.read(path)
            XCTAssertTrue((source as String).contains(".pausesSessionReplay("), "\(path) must pause replay")
        }
    }
}
