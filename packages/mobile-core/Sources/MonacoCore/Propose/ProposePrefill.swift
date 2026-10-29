public struct ProposePrefill: Hashable, Sendable {
    public var amountMicros: Int64?
    public var cabalID: String?

    public init(amountMicros: Int64? = nil, cabalID: String? = nil) {
        self.amountMicros = amountMicros
        self.cabalID = cabalID
    }
}
