import Foundation
import MonacoAPI

public enum OnboardingStep: String, Equatable, Sendable {
    case phone
    case socials
}

public struct OnboardingAPI: Sendable {
    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func linkPhone(submission: IdempotentSubmission) async throws -> SessionProfile {
        let data = try await api.sessionSubmit(submission, payload: Empty(), operation: "postOnboardingPhone") {
            client, key in
            _ = try await client.postOnboardingPhone(.init(headers: .init(idempotencyKey: key))).ok
        }
        return try SessionProfile(json: data)
    }

    public func linkSocials(submission: IdempotentSubmission) async throws -> SessionProfile {
        let data = try await api.sessionSubmit(submission, payload: Empty(), operation: "postOnboardingSocials") {
            client, key in
            _ = try await client.postOnboardingSocials(.init(headers: .init(idempotencyKey: key))).ok
        }
        return try SessionProfile(json: data)
    }

    public func skip(_ step: OnboardingStep, submission: IdempotentSubmission) async throws -> SessionProfile {
        let request = Components.Schemas.SkipOnboardingStep(step: step.payload)
        let data = try await api.sessionSubmit(submission, payload: request, operation: "postOnboardingSkip") {
            client, key in
            _ = try await client.postOnboardingSkip(.init(headers: .init(idempotencyKey: key), body: .json(request)))
                .ok
        }
        return try SessionProfile(json: data)
    }

    private struct Empty: Encodable, Sendable {}
}

extension OnboardingStep {
    fileprivate var payload: Components.Schemas.SkipOnboardingStep.StepPayload {
        switch self {
        case .phone: .phone
        case .socials: .socials
        }
    }
}
