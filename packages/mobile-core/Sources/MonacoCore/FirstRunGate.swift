import Foundation

public enum AccountRestriction: Equatable, Sendable {
    case banned
}

public enum FirstRunDestination: Equatable, Sendable {
    case session
    case restricted(AccountRestriction)
    case handle
    case phone
    case socials
    case app(restricted: Bool)
}

public enum FirstRunGate {
    public static func destination(
        for profile: SessionProfile?, onboardingCursor: OnboardingCursor
    ) -> FirstRunDestination {
        guard let profile else { return .session }
        if profile.accountStatus == .banned { return .restricted(.banned) }
        if profile.handle == nil { return .handle }
        if profile.authState == .created { return .phone }
        if onboardingCursor == .socials,
            profile.authState == .awaitingSocials || profile.authState == .awaitingPhone
        {
            return .socials
        }
        return .app(restricted: profile.accountStatus == .suspended)
    }
}
