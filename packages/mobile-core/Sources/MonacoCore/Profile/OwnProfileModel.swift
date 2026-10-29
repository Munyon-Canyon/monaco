import Foundation
import MonacoAPI
import Observation

public enum OwnStanding: Equatable, Sendable {
    case ranked(ratePercent: Int, right: Int, resolved: Int, rank: Int, rankedCount: Int)
    case unranked(resolved: Int, moreToRank: Int)

    init?(_ payload: Components.Schemas.PublicProfile.StandingPayload?) {
        guard let payload else { return nil }
        if let rank = payload.rank, let rate = payload.rateBps {
            self = .ranked(
                ratePercent: (Int(rate) + 50) / 100, right: Int(payload.right),
                resolved: Int(payload.resolved), rank: Int(rank), rankedCount: Int(payload.rankedCount))
        } else {
            self = .unranked(resolved: Int(payload.resolved), moreToRank: Int(payload.moreToRank))
        }
    }

    public var line: ProfileLine { ProfileCopy.standing(self) }
}

public enum OwnProfileTone: Equatable, Sendable {
    case gain
    case loss
    case flat
}

public enum OwnProfilePnL: Equatable, Sendable {
    case loading
    case failed
    case figure(micros: Int64, text: String, tone: OwnProfileTone)

    init(micros: Int64) {
        let tone: OwnProfileTone = micros > 0 ? .gain : (micros < 0 ? .loss : .flat)
        self = .figure(micros: micros, text: UsdAmountFormatter.format(signedMicros: micros), tone: tone)
    }
}

public struct OwnProfileCard: Equatable, Sendable {
    public let displayName: String
    public let handle: String?
    public let identityLine: String
    public let bio: String?
    public let photoURL: URL?

    public init(displayName: String, handle: String?, identityLine: String, bio: String?, photoURL: URL?) {
        self.displayName = displayName
        self.handle = handle
        self.identityLine = identityLine
        self.bio = bio
        self.photoURL = photoURL
    }
}

public struct OwnProfileCounts: Equatable, Sendable {
    public let followers: Int
    public let following: Int
    public let cabals: Int

    public init(followers: Int, following: Int, cabals: Int) {
        self.followers = followers
        self.following = following
        self.cabals = cabals
    }

    public var line: ProfileLine {
        ProfileCopy.counts(followers: followers, following: following, cabals: cabals)
    }
}

@Observable
@MainActor
public final class OwnProfileModel {
    public enum Phase: Equatable, Sendable {
        case idle
        case loading
        case loaded
        case failed
    }

    public private(set) var phase: Phase = .idle
    public private(set) var user: Components.Schemas.PublicProfile?

    private let api: APIClient
    private let hints: any HintSource
    private let viewerID: String
    private var generation = 0

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource, viewerID: String) {
        self.api = api
        self.hints = hints
        self.viewerID = viewerID
    }

    public var userID: String { viewerID }

    public var card: OwnProfileCard? {
        guard let user else { return nil }
        let name = user.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        let bio = user.bio?.trimmingCharacters(in: .whitespacesAndNewlines)
        return OwnProfileCard(
            displayName: name.isEmpty ? "Member" : name, handle: user.handle,
            identityLine: ProfileCopy.identityLine(handle: user.handle, joinedAt: user.createdAt),
            bio: bio?.isEmpty == false ? bio : nil, photoURL: user.photoUrl.flatMap(URL.init(string:)))
    }

    public var pnl1M: OwnProfilePnL {
        guard let user else { return phase == .failed ? .failed : .loading }
        guard let micros = user.pnl1mMicros else { return .failed }
        return OwnProfilePnL(micros: micros)
    }

    public var counts: OwnProfileCounts? {
        guard let user else { return nil }
        return OwnProfileCounts(
            followers: user.followerCount, following: user.followingCount, cabals: user.cabalCount)
    }

    public var standing: OwnStanding? { OwnStanding(user?.standing) }

    public var sessionProfile: SessionProfile? {
        guard let user else { return nil }
        return SessionProfile(
            userID: user.id, handle: user.handle, displayName: user.displayName,
            photoURL: user.photoUrl.flatMap(URL.init(string:)), authState: .onboardingCompleted,
            accountStatus: .active, loginProvider: .unknown(""), memberWalletAddress: "", phoneLinked: false,
            xUsername: nil, handleChangeableAt: nil, createdAt: user.createdAt, bio: user.bio)
    }

    public func load() async {
        generation += 1
        let issued = generation
        if user == nil { phase = .loading }
        do {
            let id = viewerID
            let next = try await api.read { try await $0.getUser(path: .init(id: id)).ok.body.json }
            guard issued == generation else { return }
            user = next
            phase = .loaded
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            if user == nil { phase = .failed }
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .user(what: "balance_changed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
