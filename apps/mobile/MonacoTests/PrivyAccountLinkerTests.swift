import AuthenticationServices
import Foundation
import PrivySDK
import Testing

import enum MonacoCore.LinkError

@testable import Monaco

struct PrivyAccountLinkerTests {
    @Test func aNumberOnAnotherPrivyUserIsLinkedElsewhere() {
        let error = ApiError.apiError(
            httpCode: 400, errorCode: "linked_to_another_user", description: "Account linked to another user")

        #expect(PrivyAccountLinker.linkError(from: error, credential: .phone) == .alreadyLinkedElsewhere)
        #expect(PrivyAccountLinker.linkError(from: error, credential: .oAuth) == .alreadyLinkedElsewhere)
    }

    @Test func aLoginMethodPrivyDisallowsIsUnavailable() {
        let error = ApiError.apiError(
            httpCode: 403, errorCode: "disallowed_login_method", description: "Login with Twitter not allowed")

        #expect(PrivyAccountLinker.linkError(from: error, credential: .oAuth) == .unavailable)
    }

    @Test func aRejectedCodeIsInvalidOnlyForAPhone() {
        let error = ApiError.apiError(httpCode: 401, errorCode: "invalid_credentials", description: "Invalid code")

        #expect(PrivyAccountLinker.linkError(from: error, credential: .phone) == .invalidCode)
        #expect(PrivyAccountLinker.linkError(from: error, credential: .oAuth) == .unknown)
    }

    @Test func aClosedWebSheetIsACancel() {
        let error = ASWebAuthenticationSessionError(.canceledLogin)

        #expect(PrivyAccountLinker.linkError(from: error, credential: .oAuth) == .cancelled)
        #expect(PrivyAccountLinker.linkError(from: CancellationError(), credential: .oAuth) == .cancelled)
    }

    @Test func aTransportFailureIsNetwork() {
        #expect(PrivyAccountLinker.linkError(from: URLError(.notConnectedToInternet), credential: .phone) == .network)
        #expect(
            PrivyAccountLinker.linkError(
                from: ApiError.networkError(responseCode: -1009, description: nil), credential: .phone) == .network)
    }

    @Test func anythingElseIsUnknown() {
        let error = ApiError.apiError(httpCode: 500, errorCode: "internal_error", description: "Something broke")

        #expect(PrivyAccountLinker.linkError(from: error, credential: .phone) == .unknown)
        #expect(PrivyAccountLinker.linkError(from: ApiError.malformedResponse, credential: .phone) == .unknown)
    }
}
