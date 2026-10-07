import MonacoAPI

public enum ReferralAttachOutcome: Equatable, Sendable {
    case attached
    case refusedClear
    case retryLater

    public static func from(problem error: APIError?) -> Self {
        guard let error else { return .attached }
        guard case .problem(let problem) = error, problem.status == 404 || problem.status == 422 else {
            return .retryLater
        }
        return .refusedClear
    }
}
