public protocol FlowOutcome: Sendable, Hashable {
    init?(code: String)
}

extension FlowOutcome {
    public init?(_ error: APIError) {
        switch error {
        case .problem(let problem):
            self.init(code: problem.code.wire)
        case .signedOut:
            self.init(code: Components.Schemas.ErrorCode.unauthorized.rawValue)
        case .transport, .accountDeleted, .inFlight, .decoding:
            return nil
        }
    }
}
