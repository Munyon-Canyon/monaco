import MonacoAPI

func flowProblem(_ code: String, status: Int = 409, message: String = "Server says no.") -> APIError {
    guard let known = Components.Schemas.ErrorCode(rawValue: code) else {
        return APIError(
            ProblemError(
                status: status, code: .unrecognized(code), message: message,
                traceID: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false))
    }
    return APIError(
        ProblemError(
            status: status, code: .known(known), message: message, traceID: "4bf92f3577b34da6a3ce929d0e0e4736",
            retryable: false))
}
