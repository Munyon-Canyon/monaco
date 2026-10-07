import Observation

@Observable
@MainActor
public final class ReferralEntryModel {
    public enum Result: Equatable, Sendable {
        case attached(toast: String)
        case failed(toast: String)
    }

    public var text = ""
    public private(set) var isSubmitting = false

    private let attacher: ReferralAttacher
    private let userID: String

    public init(attacher: ReferralAttacher, userID: String) {
        self.attacher = attacher
        self.userID = userID
    }

    public var canSubmit: Bool {
        !isSubmitting && !text.allSatisfy(\.isWhitespace)
    }

    public func submit() async -> Result? {
        guard canSubmit else { return nil }
        guard let code = ReferralCode(text) ?? ReferralLink.parse(text) else {
            return .failed(toast: ReferralCopy.invalidCode)
        }
        isSubmitting = true
        defer { isSubmitting = false }
        let result = await attacher.attach(code, userID: userID)
        switch result.outcome {
        case .attached: return .attached(toast: result.toast)
        case .refusedClear, .retryLater: return .failed(toast: result.toast)
        }
    }
}
