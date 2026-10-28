import MonacoAPI
import MonacoFlows

extension Flow10Outcome: FlowOutcome {}
extension Flow10aOutcome: FlowOutcome {}

public enum VoteRefusal: Equatable, Sendable {
    case closed(message: String)
    case notAVoter(message: String)
    case message(String)

    public init(_ error: APIError) {
        let message = ToastCopy.message(for: error)
        switch Flow10Outcome(error) {
        case .proposalClosed:
            self = .closed(message: message)
        case .notAVoter:
            self = .notAVoter(message: message)
        case .unauthorized, .proposalNotFound, .ok, .interrupted, nil:
            self = .message(message)
        }
    }

    public var message: String {
        switch self {
        case .closed(let message), .notAVoter(let message), .message(let message): message
        }
    }

    public var reloads: Bool {
        switch self {
        case .closed, .notAVoter: true
        case .message: false
        }
    }
}

public enum UndoRefusal: Equatable, Sendable {
    case expired(message: String)
    case voteNotFound(message: String)
    case closed(message: String)
    case message(String)

    public init(_ error: APIError) {
        let message = ToastCopy.message(for: error)
        switch Flow10aOutcome(error) {
        case .undoExpired:
            self = .expired(message: message)
        case .voteNotFound:
            self = .voteNotFound(message: message)
        case .proposalClosed:
            self = .closed(message: message)
        case .unauthorized, .proposalNotFound, .notAVoter, .ok, nil:
            self = .message(message)
        }
    }

    public var message: String {
        switch self {
        case .expired(let message), .voteNotFound(let message), .closed(let message), .message(let message): message
        }
    }

    public var reloads: Bool {
        switch self {
        case .voteNotFound, .closed: true
        case .expired, .message: false
        }
    }
}
