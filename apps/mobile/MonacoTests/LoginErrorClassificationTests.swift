import AuthenticationServices
import Foundation
import MonacoCore
import PrivySDK
import Testing

@testable import Monaco

/// How Privy failures are read. The rule under test: only a real "no session" answer
/// may sign the user out — a request that failed on the way never does.
struct LoginErrorClassificationTests {
    @Test func offlineIsNeverTreatedAsSignedOut() {
        #expect(!PrivyAuthService.isSignedOutError(URLError(.notConnectedToInternet)))
        #expect(!PrivyAuthService.isSignedOutError(URLError(.timedOut)))
        #expect(!PrivyAuthService.isSignedOutError(ApiError.networkError(responseCode: -1009, description: nil)))
        #expect(!PrivyAuthService.isSignedOutError(ApiError.malformedResponse))
    }

    @Test func urlErrorsMapToOffline() {
        #expect(PrivyAuthService.loginFailure(from: URLError(.notConnectedToInternet), step: .sendCode) == .offline)
        #expect(PrivyAuthService.loginFailure(from: URLError(.networkConnectionLost), step: .verifyCode) == .offline)
        let bridged = NSError(domain: NSURLErrorDomain, code: NSURLErrorTimedOut)
        #expect(PrivyAuthService.loginFailure(from: bridged, step: .verifyCode) == .offline)
    }

    @Test func privyTransportFailureWithoutHTTPStatusMapsToOffline() {
        let error = ApiError.networkError(responseCode: -1009, description: "offline")
        #expect(PrivyAuthService.loginFailure(from: error, step: .verifyCode) == .offline)
    }

    @Test func rateLimitIsRecognisedOnBothSteps() {
        let error = ApiError.apiError(httpCode: 429, errorCode: "too_many_requests", description: "Too many requests")
        #expect(PrivyAuthService.loginFailure(from: error, step: .sendCode) == .rateLimited)
        #expect(PrivyAuthService.loginFailure(from: error, step: .verifyCode) == .rateLimited)
    }

    @Test func rejectedCodeKeepsTheCodeField() {
        let error = ApiError.apiError(httpCode: 422, errorCode: "invalid_credentials", description: "Invalid code")
        let failure = PrivyAuthService.loginFailure(from: error, step: .verifyCode)
        #expect(failure == .codeRejected)
        #expect(failure.keepsCodeEntry)
    }

    @Test func aFullUserQuotaIsNotAWrongCode() {
        let error = ApiError.apiError(
            httpCode: 400, errorCode: "max_accounts_reached", description: "User limit reached")
        let failure = PrivyAuthService.loginFailure(from: error, step: .verifyCode)
        #expect(failure == .signUpsPaused)
        #expect(!failure.keepsCodeEntry)
        #expect(
            LoginFailureCopy.message(for: failure, step: .verifyCode)
                == "Sign-ups are paused right now. Try again later.")
    }

    @Test func aDisallowedLoginMethodIsReportedOnEveryStep() {
        let error = ApiError.apiError(
            httpCode: 403, errorCode: "disallowed_login_method", description: "Login with Apple not allowed")
        for step in [LoginStep.authorize, .sendCode, .verifyCode] {
            let failure = PrivyAuthService.loginFailure(from: error, step: step)
            #expect(failure == .methodUnavailable)
            #expect(LoginFailureCopy.message(for: failure, step: step) == "This sign-in method isn't available.")
        }
    }

    @Test func aSpentCodeIsOnlyWrongWhenPrivyCallsItInvalidCredentials() {
        let other = ApiError.apiError(httpCode: 400, errorCode: "something_else", description: "Nope")
        #expect(PrivyAuthService.loginFailure(from: other, step: .verifyCode) == .other(detail: "Nope"))
    }

    @Test func malformedProviderResponseFallsBackToGenericCopy() {
        let failure = PrivyAuthService.loginFailure(from: ApiError.malformedResponse, step: .verifyCode)
        #expect(failure == .other(detail: nil))
        #expect(LoginFailureCopy.message(for: failure, step: .verifyCode) == "Couldn't sign you in. Try again.")
    }

    @Test func closingTheAppleOrGoogleSheetIsACancel() {
        let google = ASWebAuthenticationSessionError(.canceledLogin)
        let apple = ASAuthorizationError(.canceled)
        #expect(PrivyAuthService.loginFailure(from: google, step: .authorize) == .cancelled)
        #expect(PrivyAuthService.loginFailure(from: apple, step: .authorize) == .cancelled)
        #expect(PrivyAuthService.loginFailure(from: CancellationError(), step: .authorize) == .cancelled)
    }

    @Test func aFailedAppleSheetIsNotACancel() {
        let failure = PrivyAuthService.loginFailure(from: ASAuthorizationError(.failed), step: .authorize)
        #expect(failure == .other(detail: nil))
        #expect(LoginFailureCopy.message(for: failure, step: .authorize) == "Couldn't sign you in. Try again.")
    }

    @Test func anOAuthNetworkFailureIsOffline() {
        #expect(PrivyAuthService.loginFailure(from: URLError(.notConnectedToInternet), step: .authorize) == .offline)
    }

    @Test func aProviderRejectionOnTheSheetIsNotACodeRejection() {
        let error = ApiError.apiError(httpCode: 403, errorCode: "oauth_denied", description: "Access denied")
        #expect(PrivyAuthService.loginFailure(from: error, step: .authorize) == .other(detail: "Access denied"))
    }

    @Test func aSheetFailureToastsWithoutPrivyCopy() {
        let error = ApiError.networkError(responseCode: 401, description: "Network Error")
        let failure = PrivyAuthService.loginFailure(from: error, step: .authorize)
        #expect(LoginFailureCopy.message(for: failure, step: .authorize) == "Couldn't sign you in. Try again.")
    }

    @Test func unknownErrorsFallBackToGenericCopy() {
        struct Boom: Error {}
        #expect(PrivyAuthService.loginFailure(from: Boom(), step: .sendCode) == .other(detail: nil))
    }
}
