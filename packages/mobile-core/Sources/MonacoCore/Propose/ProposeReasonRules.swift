import Foundation

public enum ProposeReasonRules {
    public static let counterStartsAt = 180
    public static let thesisLimit = 280

    public static func showsCounter(for text: String) -> Bool {
        text.count >= counterStartsAt
    }

    public static func limited(_ text: String) -> String {
        String(text.prefix(thesisLimit))
    }
}
