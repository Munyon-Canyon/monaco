import Foundation
import MonacoAPI

public protocol DeviceRegistering: Sendable {
    func register(token: String, environment: PushEnvironment) async throws
    func unregister(token: String) async throws
}

public struct DeviceAPI: DeviceRegistering {
    private let api: APIClient
    private let registration = IdempotentSubmission()
    private let removal = IdempotentSubmission()

    public init(api: APIClient) {
        self.api = api
    }

    public func register(token: String, environment: PushEnvironment) async throws {
        let request = Components.Schemas.DeviceRegistration(token: token, environment: .init(environment))
        try await api.submit(registration, payload: request, operation: "postDevice") { client, key in
            _ = try await client.postDevice(.init(headers: .init(idempotencyKey: key), body: .json(request))).noContent
        }
    }

    public func unregister(token: String) async throws {
        try await api.submit(removal, payload: token, operation: "deleteDevice") { client, key in
            let input = Operations.DeleteDevice.Input(path: .init(token: token), headers: .init(idempotencyKey: key))
            _ = try await client.deleteDevice(input).noContent
        }
    }
}

extension Components.Schemas.DeviceEnvironment {
    init(_ environment: PushEnvironment) {
        switch environment {
        case .sandbox: self = .sandbox
        case .production: self = .production
        }
    }
}
