import Foundation
@_exported import HTTPTypes
@_exported import OpenAPIRuntime
import OpenAPIURLSession

/// The one way the app calls the backend. Reads go through `read`, money writes through
/// `submit`; both throw only `APIError`.
public struct APIClient: Sendable {
    let client: Client

    public init(
        serverURL: URL,
        tokens: any AccessTokenProvider,
        transport: any ClientTransport = URLSessionTransport()
    ) {
        self.init(serverURL: serverURL, tokens: tokens, transport: transport, clock: ContinuousClock())
    }

    init(serverURL: URL, tokens: any AccessTokenProvider, transport: any ClientTransport, clock: some Clock<Duration>) {
        client = Client(
            serverURL: serverURL,
            transport: transport,
            middlewares: [
                TimeoutMiddleware(clock: clock),
                HeadersMiddleware(accessToken: { try await tokens.accessToken() }),
                ProblemMiddleware(),
                RefreshMiddleware(tokens: tokens),
            ]
        )
    }

    public func read<Output>(_ call: @Sendable (Client) async throws -> Output) async throws -> Output {
        do {
            return try await call(client)
        } catch {
            throw APIError(error)
        }
    }

    /// Runs one attempt of `submission`: `call` receives the `Idempotency-Key` to pass as
    /// `headers: .init(idempotencyKey:)`. Unwrap the expected response inside `call`
    /// (`.created.body.json`), so an unexpected answer throws and keeps the key rather than
    /// returning and dropping it.
    ///
    /// The key is reused while `operation` and `payload` stay the same and the server has
    /// not answered finally (see `APIError.isFinalAnswer`), so a retry replays instead of
    /// moving money twice.
    public func submit<Output>(
        _ submission: IdempotentSubmission,
        payload: some Encodable & Sendable,
        operation: String,
        _ call: @Sendable (Client, String) async throws -> Output
    ) async throws -> Output {
        let key: String
        do {
            key = submission.key(fingerprint: try Self.fingerprint(operation: operation, payload: payload))
        } catch {
            throw APIError(error)
        }
        do {
            let output = try await call(client, key)
            submission.record(final: true, forKey: key)
            return output
        } catch {
            let apiError = APIError(error)
            submission.record(final: apiError.isFinalAnswer, forKey: key)
            throw apiError
        }
    }

    static func fingerprint(operation: String, payload: some Encodable) throws -> Data {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        var data = Data(operation.utf8)
        data.append(0)
        data.append(try encoder.encode(payload))
        return data
    }
}
