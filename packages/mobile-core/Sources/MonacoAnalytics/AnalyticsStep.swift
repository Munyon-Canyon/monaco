public protocol AnalyticsStepName: RawRepresentable, CaseIterable, Sendable where RawValue == String {
    static var flow: AnalyticsFlow { get }
}

public struct AnalyticsStep: Hashable, Sendable {
    public let flow: AnalyticsFlow
    public let name: String

    public init<Step: AnalyticsStepName>(_ step: Step) {
        flow = Step.flow
        name = Step.flow == .referral ? step.rawValue : "\(Step.flow.rawValue)_\(step.rawValue)"
    }

    public static func onboarding(_ step: Onboarding) -> AnalyticsStep { AnalyticsStep(step) }
    public static func cryptoDeposit(_ step: CryptoDeposit) -> AnalyticsStep { AnalyticsStep(step) }
    public static func cardDeposit(_ step: CardDeposit) -> AnalyticsStep { AnalyticsStep(step) }
    public static func joinCabal(_ step: JoinCabal) -> AnalyticsStep { AnalyticsStep(step) }
    public static func propose(_ step: Propose) -> AnalyticsStep { AnalyticsStep(step) }
    public static func vote(_ step: Vote) -> AnalyticsStep { AnalyticsStep(step) }
    public static func cashOut(_ step: CashOut) -> AnalyticsStep { AnalyticsStep(step) }
    public static func withdraw(_ step: Withdraw) -> AnalyticsStep { AnalyticsStep(step) }
    public static func feed(_ step: Feed) -> AnalyticsStep { AnalyticsStep(step) }
    public static func social(_ step: Social) -> AnalyticsStep { AnalyticsStep(step) }
    public static func chat(_ step: Chat) -> AnalyticsStep { AnalyticsStep(step) }
    public static func referral(_ step: Referral) -> AnalyticsStep { AnalyticsStep(step) }

    public static var all: [AnalyticsStep] {
        Onboarding.allCases.map(AnalyticsStep.init) + CryptoDeposit.allCases.map(AnalyticsStep.init)
            + CardDeposit.allCases.map(AnalyticsStep.init) + JoinCabal.allCases.map(AnalyticsStep.init)
            + Propose.allCases.map(AnalyticsStep.init) + Vote.allCases.map(AnalyticsStep.init)
            + CashOut.allCases.map(AnalyticsStep.init) + Withdraw.allCases.map(AnalyticsStep.init)
            + Feed.allCases.map(AnalyticsStep.init) + Social.allCases.map(AnalyticsStep.init)
            + Chat.allCases.map(AnalyticsStep.init) + Referral.allCases.map(AnalyticsStep.init)
    }

    public enum Onboarding: String, AnalyticsStepName {
        case loginStarted = "login_started"
        case loginCompleted = "login_completed"
        case phoneShown = "phone_shown"
        case phoneVerified = "phone_verified"
        case phoneSkipped = "phone_skipped"
        case xShown = "x_shown"
        case xLinked = "x_linked"
        case xSkipped = "x_skipped"
        case contactsPermission = "contacts_permission"
        case followSuggestionsShown = "follow_suggestions_shown"
        case firstFollow = "first_follow"
        public static var flow: AnalyticsFlow { .onboarding }
    }

    public enum CryptoDeposit: String, AnalyticsStepName {
        case depositOpened = "deposit_opened"
        case cryptoSelected = "crypto_selected"
        case addressCopied = "address_copied"
        public static var flow: AnalyticsFlow { .cryptoDeposit }
    }

    public enum CardDeposit: String, AnalyticsStepName {
        case depositOpened = "deposit_opened"
        case cardSelected = "card_selected"
        case pageOpened = "page_opened"
        case privyAuth = "privy_auth"
        case fundConfirmed = "fund_confirmed"
        case fundCancelled = "fund_cancelled"
        public static var flow: AnalyticsFlow { .cardDeposit }
    }

    public enum JoinCabal: String, AnalyticsStepName {
        case cabalViewed = "cabal_viewed"
        case joinTapped = "join_tapped"
        case requestSent = "request_sent"
        case joined
        case fundSheetOpened = "fund_sheet_opened"
        public static var flow: AnalyticsFlow { .joinCabal }
    }

    public enum Propose: String, AnalyticsStepName {
        case proposeOpened = "propose_opened"
        case assetSelected = "asset_selected"
        case amountEntered = "amount_entered"
        case thesisEntered = "thesis_entered"
        case submitted
        public static var flow: AnalyticsFlow { .propose }
    }

    public enum Vote: String, AnalyticsStepName {
        case proposalViewed = "proposal_viewed"
        case voteCast = "vote_cast"
        public static var flow: AnalyticsFlow { .vote }
    }

    public enum CashOut: String, AnalyticsStepName {
        case cashOutOpened = "cash_out_opened"
        case amountEntered = "amount_entered"
        case confirmed
        public static var flow: AnalyticsFlow { .cashOut }
    }

    public enum Withdraw: String, AnalyticsStepName {
        case withdrawOpened = "withdraw_opened"
        case addressEntered = "address_entered"
        case confirmed
        public static var flow: AnalyticsFlow { .withdraw }
    }

    public enum Feed: String, AnalyticsStepName {
        case feedOpened = "feed_opened"
        case itemOpened = "item_opened"
        case commentOpened = "comment_opened"
        case commentPosted = "comment_posted"
        public static var flow: AnalyticsFlow { .feed }
    }

    public enum Social: String, AnalyticsStepName {
        case profileViewed = "profile_viewed"
        case followTapped = "follow_tapped"
        case suggestionShown = "suggestion_shown"
        case suggestionFollowed = "suggestion_followed"
        case suggestionDismissed = "suggestion_dismissed"
        public static var flow: AnalyticsFlow { .social }
    }

    public enum Chat: String, AnalyticsStepName {
        case chatOpened = "chat_opened"
        case messageSent = "message_sent"
        public static var flow: AnalyticsFlow { .chat }
    }

    public enum Referral: String, AnalyticsStepName {
        case invitePasteShown = "invite_paste_shown"
        case invitePasted = "invite_pasted"
        case inviteSkipped = "invite_skipped"
        public static var flow: AnalyticsFlow { .referral }
    }
}
