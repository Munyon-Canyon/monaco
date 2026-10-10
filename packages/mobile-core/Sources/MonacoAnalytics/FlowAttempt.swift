import Foundation

public struct FlowAttempt: Equatable, Sendable {
    private var ids: [AnalyticsFlow: UUID] = [:]

    public init() {}

    @discardableResult
    public mutating func start(_ flow: AnalyticsFlow, id: UUID = UUID()) -> String {
        ids[flow] = id
        return Self.format(id)
    }

    public func flowID(for flow: AnalyticsFlow) -> String? {
        ids[flow].map(Self.format)
    }

    public static func format(_ id: UUID) -> String {
        id.uuidString.lowercased()
    }
}
