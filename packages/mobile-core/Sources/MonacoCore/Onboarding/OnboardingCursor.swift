import Foundation

public enum OnboardingCursor: Equatable, Sendable {
    case start
    case socials
    case finished

    public func advanced(past step: FirstRunDestination) -> OnboardingCursor {
        switch step {
        case .phone: .socials
        case .socials: .finished
        case .session, .restricted, .handle, .app: self
        }
    }
}
