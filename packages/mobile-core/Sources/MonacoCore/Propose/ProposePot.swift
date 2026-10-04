import Foundation

public struct ProposePot: Equatable, Sendable {
    public let id: String
    public let name: String
    public let totalMicros: Int64

    public init(id: String, name: String, totalMicros: Int64) {
        self.id = id
        self.name = name
        self.totalMicros = totalMicros
    }
}
