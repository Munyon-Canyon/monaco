public struct NoopAnalyticsSink: AnalyticsSink {
    public init() {}

    public func identify(userID: String, properties: [String: AnalyticsValue]) {}
    public func capture(_ name: String, properties: [String: AnalyticsValue]) {}
    public func screen(_ name: String) {}
    public func reset() {}
}
