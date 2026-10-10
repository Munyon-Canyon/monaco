public enum AnalyticsFlow: String, CaseIterable, Sendable {
    case onboarding
    case cryptoDeposit = "crypto_deposit"
    case cardDeposit = "card_deposit"
    case joinCabal = "join_cabal"
    case propose
    case vote
    case cashOut = "cash_out"
    case withdraw
    case feed
    case social
    case chat
    case referral
}
