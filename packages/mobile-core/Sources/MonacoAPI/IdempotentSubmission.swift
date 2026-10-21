import Foundation

/// The `Idempotency-Key` for one money action the user confirmed.
///
/// The backend runs a money POST at most once per key, so the key has to mean "this
/// submission": it is minted when the request is first sent, reused for every retry while
/// the outcome is unknown (timeout, dropped connection, 5xx, the first attempt still
/// running), and dropped once the server gives a final answer. A changed payload is a new
/// submission and gets a new key.
///
/// Screens own one instance per money action (SwiftUI `@State`) and hand it to the API
/// client, which does the bookkeeping. API clients are created ad hoc, so the key cannot
/// live in them.
public final class IdempotentSubmission: @unchecked Sendable {
    public static let keyHeader = "Idempotency-Key"

    private let lock = NSLock()
    private let makeKey: @Sendable () -> String
    private var key: String?
    private var fingerprint: Data?
    private var context: Data?
    private let journal: Slot?

    private struct Slot {
        let store: PendingIntentJournal
        let accountID: String
        let slot: String

        func save(key: String, fingerprint: Data, context: Data?) {
            store.save(
                PendingIntent(
                    accountID: accountID, slot: slot, key: key, fingerprint: fingerprint, context: context))
        }

        func clear() {
            store.remove(accountID: accountID, slot: slot)
        }
    }

    /// - Parameter makeKey: key source, replaced in tests for deterministic keys.
    public init(makeKey: @escaping @Sendable () -> String = { UUID().uuidString.lowercased() }) {
        self.makeKey = makeKey
        journal = nil
    }

    public init(
        journal: PendingIntentJournal, accountID: String, slot: String,
        makeKey: @escaping @Sendable () -> String = { UUID().uuidString.lowercased() }
    ) {
        self.makeKey = makeKey
        self.journal = Slot(store: journal, accountID: accountID, slot: slot)
        if let saved = journal.intent(accountID: accountID, slot: slot), let savedKey = saved.key,
            let savedFingerprint = saved.fingerprint
        {
            key = savedKey
            fingerprint = savedFingerprint
            context = saved.context
        }
    }

    public func stage(_ value: some Encodable) {
        let data = try? JSONEncoder().encode(value)
        lock.lock()
        defer { lock.unlock() }
        context = data
    }

    public func pending<Value: Decodable>(_ type: Value.Type) -> Value? {
        lock.lock()
        let data = key == nil ? nil : context
        lock.unlock()
        return data.flatMap { try? JSONDecoder().decode(type, from: $0) }
    }

    /// True while a submission is waiting for a final answer. A retry of the same payload
    /// is a replay the backend has already seen; a changed payload is a second submission,
    /// so a screen should say so before it lets the member edit the amount.
    ///
    /// This is a snapshot taken under the lock, not a reservation. The key is minted inside
    /// the send, so it reads false between the member's tap and the request being built: a
    /// screen that gates an edit on it must read it on the same actor that owns the
    /// submission (the `@MainActor` screen that drives the send), or it can observe the gap
    /// and let the edit through while a send is starting.
    public var hasPendingKey: Bool {
        lock.lock()
        defer { lock.unlock() }
        return key != nil
    }

    /// The key to send for a request whose operation and body hash to `requestFingerprint`:
    /// the pending one when it repeats the pending submission, a fresh one otherwise.
    public func key(fingerprint requestFingerprint: Data) -> String {
        lock.lock()
        defer { lock.unlock() }
        if let key, fingerprint == requestFingerprint {
            return key
        }
        let fresh = makeKey()
        key = fresh
        fingerprint = requestFingerprint
        journal?.save(key: fresh, fingerprint: requestFingerprint, context: context)
        return fresh
    }

    /// Records the server's answer to a request sent under `sentKey`. A non-final answer
    /// keeps the key so the next attempt is recognised as a retry, and so does a failed send
    /// that needs no call at all. A final answer for a superseded key changes nothing.
    public func record(final: Bool, forKey sentKey: String) {
        guard final else { return }
        lock.lock()
        defer { lock.unlock() }
        guard key == sentKey else { return }
        key = nil
        fingerprint = nil
        context = nil
        journal?.clear()
    }
}
