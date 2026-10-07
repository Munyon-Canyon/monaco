import AuthenticationServices
import Foundation
import MonacoAPI
import MonacoCore
import PrivySDK
import os

actor PrivyAccountLinker: AccountLinking {
    private let privy: any Privy
    private let api: APIClient
    private var codeSentTo: String?

    init(privy: any Privy, api: APIClient) {
        self.privy = privy
        self.api = api
    }

    func sendPhoneCode(e164: String) async throws {
        do {
            try await privy.sms.sendCode(to: e164)
            codeSentTo = e164
        } catch {
            throw Self.failure(error, step: "Send link code", credential: .phone)
        }
    }

    func linkPhone(code: String) async throws {
        guard let codeSentTo else { throw LinkError.unknown }
        do {
            _ = try await privy.sms.linkWithCode(code, sentTo: codeSentTo)
        } catch {
            throw Self.failure(error, step: "Link phone", credential: .phone)
        }
    }

    func linkX() async throws {
        #if DEBUG
        if ProcessInfo.processInfo.environment["MONACO_FAKE_X"] == "1" {
            try await linkFakeX()
            return
        }
        #endif
        do {
            _ = try await privy.oAuth.link(with: .twitter, appUrlScheme: "monaco")
        } catch {
            throw Self.failure(error, step: "Link X", credential: .oAuth)
        }
    }

    #if DEBUG
    private struct FakeXLink: Encodable, Sendable {}

    private func linkFakeX() async throws {
        do {
            try await api.submit(IdempotentSubmission(), payload: FakeXLink(), operation: "postDevXLink") {
                client, key in
                _ = try await client.postDevXLink(.init(headers: .init(idempotencyKey: key))).noContent
            }
        } catch {
            throw Self.failure(error, step: "Link fake X", credential: .oAuth)
        }
    }
    #endif

    enum Credential {
        case phone
        case oAuth
    }

    private static func failure(_ error: any Error, step: String, credential: Credential) -> LinkError {
        let mapped = linkError(from: error, credential: credential)
        let detail = LogRedaction.phoneNumbers(in: String(describing: error))
        AppLogger.linking.error(
            "\(step, privacy: .public) failed as \(String(describing: mapped), privacy: .public): \(detail, privacy: .public)"
        )
        return mapped
    }

    nonisolated static func linkError(from error: any Error, credential: Credential) -> LinkError {
        if let linkError = error as? LinkError { return linkError }
        if error is CancellationError { return .cancelled }
        if let webError = error as? ASWebAuthenticationSessionError, webError.code == .canceledLogin {
            return .cancelled
        }
        if error is URLError || (error as NSError).domain == NSURLErrorDomain { return .network }
        if let apiError = error as? ApiError { return linkError(from: apiError, credential: credential) }
        if let privyError = error as? PrivyError, case .authenticationFailure(let reason) = privyError.errorCode {
            return linkError(from: reason, credential: credential)
        }
        return .unknown
    }

    private nonisolated static func linkError(from apiError: ApiError, credential: Credential) -> LinkError {
        switch apiError {
        case .apiError(_, let errorCode, let description):
            linkError(errorCode: errorCode, description: description, credential: credential)
        case .networkError(let responseCode, _):
            (400..<600).contains(responseCode) ? .unknown : .network
        case .couldNotConstructRequest, .decodingError, .malformedResponse:
            .unknown
        @unknown default:
            .unknown
        }
    }

    private nonisolated static func linkError(
        from reason: PrivyErrorCode.AuthenticationFailureReason, credential: Credential
    ) -> LinkError {
        switch reason {
        case .accountTransferRequired:
            .alreadyLinkedElsewhere
        case .incorrectCredentials:
            credential == .phone ? .invalidCode : .unknown
        case .failureDuringAuthentication(let underlying):
            linkError(from: underlying, credential: credential)
        default:
            .unknown
        }
    }

    private nonisolated static func linkError(errorCode: String, description: String, credential: Credential)
        -> LinkError
    {
        let text = "\(errorCode) \(description)".lowercased()
        if credential == .phone, text.contains("cannot_link_more_of_type") { return .alreadyHasPhone }
        if text.contains("linked_to_another_user") || text.contains("another user") || text.contains("another account")
            || text.contains("already linked") || text.contains("already exists")
        {
            return .alreadyLinkedElsewhere
        }
        if credential == .phone, text.contains("invalid_credentials") || text.contains("invalid code") {
            return .invalidCode
        }
        return .unknown
    }
}
