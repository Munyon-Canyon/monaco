public protocol AnalyticsSink: Sendable {
    func identify(userID: String, properties: [String: AnalyticsValue])
    func capture(_ name: String, properties: [String: AnalyticsValue])
    func screen(_ name: String)
    func reset()
}
