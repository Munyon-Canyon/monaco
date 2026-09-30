import Foundation
import MonacoAPI

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

extension IdempotentSubmission {
    /// Set by the backend on responses it did not produce by running the request.
    public static let statusHeader = "Idempotency-Status"
    /// `statusHeader` value on the 409 sent while the first attempt is still running.
    public static let inProgressStatus = "in_progress"

    /// The key to send with `request`: the pending one when `request` repeats the pending
    /// submission (same method, URL and body), a fresh one otherwise.
    public func key(for request: URLRequest) -> String {
        key(fingerprint: Self.fingerprint(of: request))
    }

    /// Records the server's answer to a request sent under `key`. A failed send (no
    /// response at all) needs no call: the key simply stays pending.
    public func record(response: URLResponse, forKey sentKey: String) {
        guard let http = response as? HTTPURLResponse else { return }
        record(final: Self.isFinal(http), forKey: sentKey)
    }

    /// 2xx and 4xx are the request's result. 5xx is never stored by the backend, 401 and 429
    /// are answered before the key is claimed, and an in-progress 409 means the first attempt
    /// has not finished; all of those must be retried under the same key.
    static func isFinal(_ response: HTTPURLResponse) -> Bool {
        switch response.statusCode {
        case 401, 429:
            return false
        case 409:
            return response.value(forHTTPHeaderField: statusHeader) != inProgressStatus
        case 200..<500:
            return true
        default:
            return false
        }
    }

    private static func fingerprint(of request: URLRequest) -> Data {
        var data = Data((request.httpMethod ?? "").utf8)
        data.append(0)
        data.append(Data((request.url?.absoluteString ?? "").utf8))
        data.append(0)
        data.append(request.httpBody ?? Data())
        return data
    }
}

extension MonacoHTTPTransport {
    /// Sends `request`, under `submission`'s idempotency key when one is given. The key is
    /// per submission and survives retries; the `X-Request-Id` stays per attempt.
    public func send(
        _ request: URLRequest,
        route: String? = nil,
        timeout: TimeInterval? = nil,
        submission: IdempotentSubmission?
    ) async throws -> MonacoHTTPResponse {
        guard let submission else {
            return try await send(request, route: route, timeout: timeout)
        }
        var request = request
        let key = submission.key(for: request)
        request.setValue(key, forHTTPHeaderField: IdempotentSubmission.keyHeader)
        let result = try await send(request, route: route, timeout: timeout)
        submission.record(response: result.response, forKey: key)
        return result
    }

    /// `data(for:)` for a money POST: same request id and telemetry, plus the idempotency key.
    public func data(for request: URLRequest, submission: IdempotentSubmission) async throws -> (Data, URLResponse) {
        let result = try await send(request, submission: submission)
        return (result.data, result.response)
    }

    /// JSON bodies of money POSTs are encoded with sorted keys: a retry must produce the
    /// same bytes, or it would be taken for a new submission.
    public static func idempotentBodyEncoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        return encoder
    }
}
