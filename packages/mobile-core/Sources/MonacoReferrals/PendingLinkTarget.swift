import Foundation

public struct PendingLinkTarget: Codable, Equatable, Sendable {
    public enum Kind: String, Codable, Sendable {
        case join
        case vote
    }

    public let kind: Kind
    public let code: String
    public let cabalID: String
    public let proposalID: String?
    public let joinTapped: Bool
    public let savedAt: Date

    public init(kind: Kind, code: String, cabalID: String, proposalID: String?, joinTapped: Bool, savedAt: Date) {
        self.kind = kind
        self.code = code
        self.cabalID = cabalID
        self.proposalID = proposalID
        self.joinTapped = joinTapped
        self.savedAt = savedAt
    }
}

public struct PendingLinkTargetStore {
    public static let key = "monaco.pendingLinkTarget"

    private let store: any KeyValueStoring

    public init(store: any KeyValueStoring = UserDefaults.standard) {
        self.store = store
    }

    public func save(_ target: PendingLinkTarget) {
        store.set(try? JSONEncoder().encode(target), forKey: Self.key)
    }

    public func save(_ target: ReferralLinkTarget, now: Date) {
        guard let cabalID = target.cabalID else { return }
        save(
            PendingLinkTarget(
                kind: target.proposalID == nil ? .join : .vote, code: target.code.value, cabalID: cabalID,
                proposalID: target.proposalID, joinTapped: false, savedAt: now))
    }

    public func peek(now: Date) -> PendingLinkTarget? {
        guard let data = store.data(forKey: Self.key) else { return nil }
        guard let target = try? JSONDecoder().decode(PendingLinkTarget.self, from: data),
            now.timeIntervalSince(target.savedAt) <= PendingReferralStore.attributionWindow
        else {
            store.removeObject(forKey: Self.key)
            return nil
        }
        return target
    }

    public func take(now: Date) -> PendingLinkTarget? {
        let target = peek(now: now)
        if target != nil { store.removeObject(forKey: Self.key) }
        return target
    }

    public func takeJoin(cabalID: String, now: Date) -> Bool {
        guard let target = peek(now: now), target.cabalID == cabalID, target.joinTapped else { return false }
        store.removeObject(forKey: Self.key)
        return true
    }
}
