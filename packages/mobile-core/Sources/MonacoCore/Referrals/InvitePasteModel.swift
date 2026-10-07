import Foundation
import Observation

@Observable
@MainActor
public final class InvitePasteModel {
    public enum Result: Equatable, Sendable {
        case added
        case notAnInvite

        public var toast: String {
            switch self {
            case .added: ReferralCopy.inviteAdded
            case .notAnInvite: ReferralCopy.invalidLink
            }
        }
    }

    private let gate: InvitePasteGate
    private let pending: PendingReferralStore
    private let now: @Sendable () -> Date
    private let track: @MainActor (ReferralAppEvent) -> Void

    public init(
        store: any KeyValueStoring, now: @escaping @Sendable () -> Date,
        track: @escaping @MainActor (ReferralAppEvent) -> Void
    ) {
        gate = InvitePasteGate(store: store)
        pending = PendingReferralStore(store: store)
        self.now = now
        self.track = track
    }

    public func shouldOffer(isFirstLaunch: Bool, clipboardHasProbableURL: () async -> Bool) async -> Bool {
        let hasPending = pending.load(now: now()) != nil
        guard
            gate.shouldOffer(
                isFirstLaunch: isFirstLaunch, isSignedIn: false, clipboardHasProbableURL: true,
                hasPendingReferral: hasPending)
        else { return false }
        let offer = gate.shouldOffer(
            isFirstLaunch: isFirstLaunch, isSignedIn: false, clipboardHasProbableURL: await clipboardHasProbableURL(),
            hasPendingReferral: hasPending)
        if offer { track(.invitePasteShown) }
        return offer
    }

    public func paste(_ url: URL) -> Result {
        gate.markOffered()
        guard let code = ReferralLink.parse(url) else { return .notAnInvite }
        do {
            try pending.save(code, source: .clipboard, at: now())
        } catch {
            return .notAnInvite
        }
        track(.invitePasted)
        return .added
    }

    public func skip() {
        gate.markOffered()
        track(.inviteSkipped)
    }
}
