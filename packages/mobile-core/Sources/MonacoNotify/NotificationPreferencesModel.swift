import MonacoAPI
import Observation

public enum NotificationCategory: Sendable, Equatable {
    case decisionRequests, repliesAndMentions
}

public struct NotificationChoices: Sendable, Equatable {
    public var decisionRequests: Bool
    public var repliesAndMentions: Bool

    public init(decisionRequests: Bool = true, repliesAndMentions: Bool = true) {
        self.decisionRequests = decisionRequests
        self.repliesAndMentions = repliesAndMentions
    }

    public subscript(category: NotificationCategory) -> Bool {
        get {
            switch category {
            case .decisionRequests: decisionRequests
            case .repliesAndMentions: repliesAndMentions
            }
        }
        set {
            switch category {
            case .decisionRequests: decisionRequests = newValue
            case .repliesAndMentions: repliesAndMentions = newValue
            }
        }
    }

    init(_ preferences: Components.Schemas.NotificationPreferences) {
        self.init(
            decisionRequests: preferences.decisionRequests, repliesAndMentions: preferences.repliesAndMentions)
    }

    var body: Components.Schemas.NotificationPreferences {
        .init(decisionRequests: decisionRequests, repliesAndMentions: repliesAndMentions)
    }
}

@Observable
@MainActor
public final class NotificationPreferencesModel {
    public private(set) var choices: NotificationChoices?
    public private(set) var isSaving = false

    private let api: APIClient
    private let submission = IdempotentSubmission()

    public init(api: APIClient) {
        self.api = api
    }

    public var canChange: Bool { choices != nil && !isSaving }

    public func load() async {
        guard !isSaving else { return }
        do {
            let loaded = try await api.read { try await $0.getNotificationPreferences().ok.body.json }
            guard !isSaving else { return }
            choices = NotificationChoices(loaded)
        } catch {
            return
        }
    }

    public func set(_ category: NotificationCategory, to enabled: Bool) async -> APIError? {
        guard let before = choices, !isSaving, before[category] != enabled else { return nil }
        var after = before
        after[category] = enabled
        choices = after
        isSaving = true
        defer { isSaving = false }
        let sent = after.body
        do {
            let stored = try await api.submit(
                submission, payload: sent, operation: Operations.PutNotificationPreferences.id
            ) { client, key in
                try await client.putNotificationPreferences(
                    headers: .init(idempotencyKey: key), body: .json(sent)
                ).ok.body.json
            }
            choices = NotificationChoices(stored)
            return nil
        } catch {
            choices = before
            return APIError(error)
        }
    }
}
