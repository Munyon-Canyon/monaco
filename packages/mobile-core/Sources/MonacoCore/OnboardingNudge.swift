public enum OnboardingNudge: Equatable, Sendable {
    case addPhone(String)
    case linkX(String)

    public var message: String {
        switch self {
        case .addPhone(let message), .linkX(let message): message
        }
    }
}

public func nudge(for profile: SessionProfile) -> OnboardingNudge? {
    guard profile.accountStatus == .active else { return nil }
    switch profile.authState {
    case .awaitingPhone: return .addPhone("Add your number to find friends")
    case .awaitingSocials: return .linkX("Connect X to find people you follow")
    case .created, .onboardingCompleted, .unknown: return nil
    }
}
