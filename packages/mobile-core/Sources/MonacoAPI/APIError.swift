import Foundation
import OpenAPIRuntime

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Every failure an `APIClient` call throws. Show `ToastCopy.message(for:)`; switch on
/// `ProblemError.code` only where a flow branches.
public enum APIError: Error, Sendable, Hashable {
    case problem(ProblemError)
    case transport(URLError)
    case signedOut
    case missingAccessToken(String)
    case accountDeleted
    /// The first attempt of this submission is still running. Retry with the same
    /// `IdempotentSubmission`.
    case inFlight
    case decoding(String)

    public init(_ error: any Error) {
        switch error {
        case let error as APIError:
            self = error
        case let clientError as ClientError:
            self.init(clientError.underlyingError)
        case let problem as ProblemError:
            self.init(problem)
        case let urlError as URLError:
            self = .transport(urlError)
        default:
            self = .decoding(String(describing: error))
        }
    }

    init(_ problem: ProblemError) {
        switch problem.code.wire {
        case "account_deleted":
            self = .accountDeleted
        case Components.Schemas.ErrorCode.idempotencyInFlight.rawValue:
            self = .inFlight
        case Components.Schemas.ErrorCode.idempotencyMismatch.rawValue:
            assertionFailure("an Idempotency-Key was reused for a different body: \(problem.message)")
            self = .problem(problem)
        default:
            self = .problem(problem)
        }
    }

    /// Whether the server gave the submission its final answer, so a retry is a new
    /// submission. 401 and 429 are answered before the key is claimed, an in-flight 409
    /// means the first attempt has not finished, and 5xx, transport errors and timeouts
    /// leave the outcome unknown.
    var isFinalAnswer: Bool {
        switch self {
        case .problem(let problem):
            return (400..<500).contains(problem.status) && problem.status != 401 && problem.status != 429
        case .accountDeleted:
            return true
        case .transport, .signedOut, .missingAccessToken, .inFlight, .decoding:
            return false
        }
    }
}
