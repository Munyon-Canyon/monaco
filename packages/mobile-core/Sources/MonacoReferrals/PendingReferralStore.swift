import Foundation

public protocol KeyValueStoring: AnyObject {
    func data(forKey key: String) -> Data?
    func bool(forKey key: String) -> Bool
    func set(_ value: Any?, forKey key: String)
    func removeObject(forKey key: String)
}

extension UserDefaults: KeyValueStoring {}

public struct PendingReferral: Equatable, Sendable {
    public let code: ReferralCode
    public let source: ReferralSource
    public let capturedAt: Date

    public init(code: ReferralCode, source: ReferralSource, capturedAt: Date) {
        self.code = code
        self.source = source
        self.capturedAt = capturedAt
    }
}

public struct PendingReferralStore {
    public static let key = "monaco.pendingReferral"
    public static let attributionWindow: TimeInterval = 7 * 24 * 60 * 60

    private struct Record: Codable {
        let code: String
        let source: ReferralSource
        let capturedAt: Date
    }

    private let store: any KeyValueStoring

    public init(store: any KeyValueStoring) {
        self.store = store
    }

    public func save(_ code: ReferralCode, source: ReferralSource, at now: Date) throws {
        let record = Record(code: code.value, source: source, capturedAt: now)
        store.set(try JSONEncoder().encode(record), forKey: Self.key)
    }

    public func load(now: Date) -> PendingReferral? {
        guard let data = store.data(forKey: Self.key) else { return nil }
        guard let record = try? JSONDecoder().decode(Record.self, from: data),
            let code = ReferralCode(record.code),
            now.timeIntervalSince(record.capturedAt) <= Self.attributionWindow
        else {
            clear()
            return nil
        }
        return PendingReferral(code: code, source: record.source, capturedAt: record.capturedAt)
    }

    public func clear() {
        store.removeObject(forKey: Self.key)
    }
}

public struct InvitePasteGate {
    public static let key = "monaco.invitePasteOffered"

    private let store: any KeyValueStoring

    public init(store: any KeyValueStoring) {
        self.store = store
    }

    public func shouldOffer(
        isFirstLaunch: Bool,
        isSignedIn: Bool,
        clipboardHasProbableURL: Bool,
        hasPendingReferral: Bool
    ) -> Bool {
        isFirstLaunch && !isSignedIn && clipboardHasProbableURL && !hasPendingReferral
            && !store.bool(forKey: Self.key)
    }

    public func markOffered() {
        store.set(true, forKey: Self.key)
    }
}
