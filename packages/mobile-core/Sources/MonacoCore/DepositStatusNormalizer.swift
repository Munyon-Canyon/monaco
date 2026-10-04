public enum DepositPolling {
    public static let sweepStatusInterval: Duration = .seconds(3)
}

public enum DepositStatusNormalizer {
    public static func isPending(_ status: String) -> Bool {
        status.lowercased() == "pending"
    }

    public static func isConfirmed(_ status: String) -> Bool {
        status.lowercased() == "confirmed"
    }

    public static func isFailed(_ status: String) -> Bool {
        let normalized = status.lowercased()
        return normalized == "failed" || normalized.hasPrefix("failed:")
    }
}
