#if DEBUG
import MonacoAPI
import MonacoCore
import MonacoFlows
import SwiftUI

enum GroupDetailSampleScenario: String, CaseIterable {
    case ready
    case noShare
    case cashOnly
    case noProposals
    case executing
    case completed
    case empty
    case loading
    case error
    case picture
    case noPicture
    case pictureNotCreator
    case pictureUploadFailure
    case paused
    case outsider
    case invited
    case votes
    case publicPreview
    case publicPending
    case publicMember
    case publicPrivate
    case info
    case infoAdmin
    case members
    case voters
    case requests
    case requestsEmpty
    case invite
    case inviteNoProposal
    case inviteMember
    case inviteLoading
    case inviteFailed
    case invitePreview
    case inviteInvalid
    case invitePending
    case fTreasury
    case fTreasuryLight
    case fTreasuryScrolled
    case fTreasuryHistory
    case fRules
    case fRulesScrolled
    case fRulesSides
    case fRuleProposal
    case fRuleProposalOpen
    case fMembers
    case fMembersScrolled
    case fMembersAll
    case fVoters
    case fMembersLarge
    case fEmpty

    static let launchArgument = "-MonacoGroupDetailSample"

    static func matching(_ arguments: [String]) -> GroupDetailSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return GroupDetailSampleScenario(rawValue: arguments[flag + 1])
    }
}

extension GroupDetailSampleScenario {
    @MainActor
    var script: SampleAPIScript {
        switch self {
        case .ready, .invitePreview, .inviteInvalid, .invitePending, .inviteMember:
            SampleAPIScript(role: "member", unreadCount: 3)
        case .fTreasury, .fTreasuryLight, .fTreasuryScrolled, .fTreasuryHistory, .fRules, .fRulesScrolled,
            .fRuleProposal:
            GroupDetailSampleData.fTreasury
        case .fMembers, .fMembersScrolled, .fVoters: GroupDetailSampleData.fMembers
        case .fMembersAll: GroupDetailSampleData.fMembersAll
        case .fMembersLarge: GroupDetailSampleData.fMembersLarge
        case .fRulesSides: GroupDetailSampleData.fRulesSides
        case .fEmpty: GroupDetailSampleData.fEmpty
        case .fRuleProposalOpen: GroupDetailSampleData.fRuleProposalOpen
        case .invite: SampleAPIScript(role: "creator", unreadCount: 3)
        case .inviteNoProposal: SampleAPIScript(role: "creator", unreadCount: 3, invitePreview: .noProposal)
        case .inviteLoading: SampleAPIScript(role: "creator", unreadCount: 3, invitePreview: .hang)
        case .inviteFailed: SampleAPIScript(role: "creator", unreadCount: 3, invitePreview: .fail)
        case .noShare: SampleAPIScript(role: "member", pot: .noShare)
        case .cashOnly: SampleAPIScript(role: "member", pot: .sampleCashOnly)
        case .noProposals: SampleAPIScript(role: "member", hidesProposals: true)
        case .executing: SampleAPIScript(role: "member", proposalStatus: .passed)
        case .completed: SampleAPIScript(role: "member", proposalStatus: .executed)
        case .empty: SampleAPIScript(mode: .empty, role: "creator")
        case .loading: SampleAPIScript(mode: .hang, role: "member")
        case .error: SampleAPIScript(mode: .offline, role: "member")
        case .picture: SampleAPIScript(role: "creator", pictureURL: GroupDetailSampleData.pictureURL)
        case .noPicture: SampleAPIScript(role: "creator")
        case .pictureNotCreator: SampleAPIScript(role: "member", pictureURL: GroupDetailSampleData.pictureURL)
        case .pictureUploadFailure:
            SampleAPIScript(role: "creator", pictureURL: GroupDetailSampleData.pictureURL, pictureWriteFails: true)
        case .paused: SampleAPIScript(role: "member", cashOut: .sampleOpsPause)
        case .outsider, .publicPreview:
            SampleAPIScript(role: nil, cabalCount: 0, pot: .sampleOutsider)
        case .invited:
            SampleAPIScript(role: nil, cabalCount: 0, pot: .sampleOutsider, invites: [GroupDetailSampleData.invite])
        case .publicPending:
            SampleAPIScript(role: nil, cabalCount: 0, pot: .sampleOutsider, accessRequest: ("request", "pending"))
        case .publicMember, .publicPrivate:
            SampleAPIScript(role: "member")
        case .info, .members, .voters: SampleAPIScript(role: "member")
        case .infoAdmin:
            SampleAPIScript(role: "creator", accessRequests: Components.Schemas.CabalAccessRequest.samples)
        case .requests:
            SampleAPIScript(role: "creator", accessRequests: Components.Schemas.CabalAccessRequest.samples)
        case .requestsEmpty: SampleAPIScript(role: "creator")
        case .votes:
            SampleAPIScript(
                role: "member", proposal: .sample(canVote: true),
                pendingVotes: [Components.Schemas.PendingVote.samples[0]], viewerCanVote: true)
        }
    }

    var routes: [any AppRoute] {
        let id = GroupDetailSampleData.cabalID
        switch self {
        case .info, .infoAdmin, .fRules, .fRulesScrolled, .fRulesSides, .fRuleProposal, .fRuleProposalOpen:
            return [CabalRoute(id: id, tab: .rules)]
        case .fTreasuryHistory: return [CabalTreasuryHistoryRoute(cabalID: id)]
        case .fMembers, .fMembersScrolled: return [CabalRoute(id: id, tab: .members)]
        case .fVoters: return [CabalRoute(id: id, tab: .voters)]
        case .members, .fMembersAll, .fMembersLarge, .requests, .requestsEmpty: return [MembersRoute(cabalID: id)]
        case .voters: return [CabalRoute(id: id, tab: .voters)]
        case .publicMember: return [CabalSampleRoute(kind: .publicMember)]
        case .publicPrivate: return [CabalSampleRoute(kind: .publicPrivate)]
        case .invite, .inviteNoProposal, .inviteLoading, .inviteFailed:
            return [CabalSampleRoute(kind: .invite(.share, role: "creator"))]
        case .inviteMember: return [CabalSampleRoute(kind: .invite(.share, role: "member"))]
        case .invitePreview: return [CabalSampleRoute(kind: .invite(.invited, role: "member"))]
        case .inviteInvalid: return [CabalSampleRoute(kind: .invite(.unavailable, role: "member"))]
        case .invitePending: return [CabalSampleRoute(kind: .invite(.requested, role: "member"))]
        default: return [CabalRoute(id: id)]
        }
    }
}

extension Components.Schemas.CabalPot {
    static var noShare: Self {
        var pot = Self.sampleInvested
        pot.me = .init(shareUnits: 0, valueMicros: 0, sliceBps: 0, netContributedMicros: 0, pnlMicros: 0)
        return pot
    }
}

enum GroupDetailSampleData {
    static let cabalID = Components.Schemas.Cabal.sample(role: nil).id

    static var invite: Components.Schemas.CabalInvite {
        var invite = Components.Schemas.CabalInvite.sample
        let cabal = Components.Schemas.Cabal.sample(role: nil)
        invite.cabal.id = cabal.id
        invite.cabal.name = cabal.name
        return invite
    }

    @MainActor
    static var fTreasury: SampleAPIScript {
        var script = SampleAPIScript(role: "admin", pictureURL: pictureURL, pot: .sampleACCapital)
        script.cabalEdit = { cabal in
            cabal.name = "AC Capital Management"
            cabal.memberCount = 8
            cabal.voterCount = 8
            cabal.openProposalCount = 1
            cabal.createdAt = Date(timeIntervalSince1970: 1_786_000_000)
            cabal.rules.threshold = "majority"
            cabal.rules.proposalExpirySeconds = 86_400
            cabal.rules.maxTradeBps = 2000
            cabal.admins = [
                .init(
                    userId: "01890a5d-ac96-774b-bcce-b302099a8071", handle: "andres", displayName: "Andres",
                    photoUrl: nil),
                .init(
                    userId: "01890a5d-ac96-774b-bcce-b302099a8072", handle: "christelle",
                    displayName: "Christelle", photoUrl: nil),
            ]
        }
        var proposal = Components.Schemas.Proposal.sample()
        proposal.id = "proposal-nvda"
        proposal.cabalName = "AC Capital Management"
        proposal.symbol = "NVDAx"
        proposal.usdcMicros = 400_000_000
        proposal.expiresAt = .now.addingTimeInterval(4 * 3_600 + 600)
        proposal.tally = .init(yes: 4, no: 1, voters: 8, needed: 5)
        script.openProposal = proposal
        let start = Date(timeIntervalSince1970: 1_790_931_600)
        script.valueHistory = Components.Schemas.CabalValueHistory(
            cabalId: Components.Schemas.Cabal.sample(role: nil).id, range: ._1m,
            points: (0..<31).map { day in
                Components.Schemas.CabalValuePoint(
                    at: start.addingTimeInterval(TimeInterval(day * 86_400)),
                    valueMicros: 20_000_000_000 - Int64(day) * 27_000_000,
                    navPerShareMicros: 1_000_000 - Int64(day) * 1_366, pnlMicros: 0)
            },
            pricesAsOf: start.addingTimeInterval(30 * 86_400))
        return script
    }

    @MainActor
    static var fEmpty: SampleAPIScript {
        var script = SampleAPIScript(
            role: "admin",
            pot: .sample(
                potValueMicros: 600_000_000, cashMicros: 600_000_000, cashWeightBps: 10_000, pnlMicros: 0, holdings: [],
                me: .init(
                    shareUnits: 200_000_000, valueMicros: 200_000_000, sliceBps: 3333,
                    netContributedMicros: 200_000_000, pnlMicros: 0)))
        script.hidesProposals = true
        script.catalog = Components.Schemas.AssetList.inCabalsSample.assets
        script.cabalEdit = { cabal in
            cabal.name = "Garage Fund"
            cabal.memberCount = 3
            cabal.voterCount = 3
            cabal.openProposalCount = 0
            cabal.createdAt = .now
        }
        return script
    }

    @MainActor
    static var fMembers: SampleAPIScript {
        var script = fTreasury
        var page = Components.Schemas.CabalMemberPage.sampleACCapital
        page.items[0].userId = Components.Schemas.Me.sample.id
        script.memberPage = page
        return script
    }

    @MainActor
    static var fMembersAll: SampleAPIScript {
        var script = fMembers
        script.accessRequests = [Components.Schemas.CabalAccessRequest.samples[0]]
        return script
    }

    @MainActor
    static var fMembersLarge: SampleAPIScript {
        var script = fMembersAll
        let joined = Date(timeIntervalSince1970: 1_790_000_000)
        let admins = Array(Components.Schemas.CabalMemberPage.sampleACCapital.items.prefix(2))
        let rest = (0..<118).map { index in
            Components.Schemas.CabalMemberRow(
                userId: "01890a5d-ac96-774b-bcce-b302099b\(String(format: "%04d", index))",
                handle: "member\(index)", displayName: "Member \(index)", photoUrl: nil, role: .member,
                canVote: index < 80, joinedAt: joined, ownershipBps: Int32(max(1, 100 - index)),
                month: .init(right: 3, resolved: 5, voted: 6, eligible: 6), crown: false)
        }
        script.memberPage = .init(
            items: admins + rest, nextCursor: nil, totals: .init(admins: 2, voters: 80, members: 38))
        return script
    }

    @MainActor
    static var fRulesSides: SampleAPIScript {
        var script = fTreasury
        let edit = script.cabalEdit
        script.cabalEdit = { cabal in
            edit?(&cabal)
            let year = Calendar.current.component(.year, from: .now)
            cabal.rules.showSides = true
            cabal.rules.showSidesSince = Calendar.current.date(
                from: DateComponents(year: year, month: 10, day: 20, hour: 12))
        }
        return script
    }

    @MainActor
    static var fRuleProposalOpen: SampleAPIScript {
        var script = fTreasury
        var proposal = Components.Schemas.Proposal.ruleSample(now: .now)
        proposal.id = "proposal-rule"
        proposal.cabalName = "AC Capital Management"
        script.openProposal = proposal
        return script
    }

    @MainActor
    static var pictureURL: String? {
        SampleImage.flat(.indigo, name: "cabal-picture")?.absoluteString
    }
}

@MainActor
struct SampleCabalPictureWriter: CabalPictureWriting {
    var alwaysFails = false

    func uploadPicture(groupId: String, imageData: Data, mimeType: String) async throws -> String? {
        try? await Task.sleep(for: .milliseconds(700))
        if alwaysFails {
            throw APIError.problem(
                ProblemError(
                    status: 413, code: .init("picture_invalid"), message: "picture must be at most 2MB",
                    traceID: "sample", retryable: false))
        }
        return GroupDetailSampleData.pictureURL
    }

    func removePicture(groupId: String) async throws -> String? {
        try? await Task.sleep(for: .milliseconds(400))
        if alwaysFails {
            throw APIError.problem(
                ProblemError(
                    status: 503, code: .init("storage_unavailable"), message: "Pictures can't be saved right now.",
                    traceID: "sample", retryable: true))
        }
        return nil
    }
}

struct GroupDetailSampleHarness: View {
    let scenario: GroupDetailSampleScenario
    @ObservedObject var auth: PrivyAuthService

    var body: some View {
        SampleAppFrame(
            auth: auth, tab: .cabals, routes: scenario.routes,
            session: scenario == .loading ? SampleAppFrame.loading : SampleAppFrame.signedIn)
    }
}

final class GroupDetailSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = GroupDetailSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        return AnyView(GroupDetailSampleHarness(scenario: scenario, auth: auth))
    }
}

final class MembersFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard Flow03Scenario.matching(arguments) != nil else { return nil }
        let scenario = GroupDetailSampleScenario.error
        SampleAPIProtocol.install(scenario.script)
        return AnyView(GroupDetailSampleHarness(scenario: scenario, auth: auth))
    }
}

nonisolated struct CabalSampleRoute: AppRoute {
    enum Kind: Hashable, Sendable {
        case publicMember
        case publicPrivate
        case invite(CabalInviteMode, role: String)
    }

    let kind: Kind

    @MainActor func destination() -> some View {
        CabalSampleHost(kind: kind)
    }
}

private struct CabalSampleHost: View {
    let kind: CabalSampleRoute.Kind

    @Environment(AppEnvironment.self) private var environment
    @State private var models = CabalScreenModels()

    private var cabalID: String { GroupDetailSampleData.cabalID }

    var body: some View {
        let (cabal, _) = models.prepared(cabalID: cabalID, environment: environment)
        content(cabal)
            .task { await cabal.load() }
    }

    @ViewBuilder private func content(_ cabal: CabalModel) -> some View {
        switch kind {
        case .publicMember:
            PublicCabalView(cabal: cabal, retry: {})
        case .publicPrivate:
            PublicCabalView(cabal: cabal, isPrivate: true, retry: {})
        case .invite(let mode, let role):
            CabalScreen(cabalID: cabalID)
                .sheet(isPresented: .constant(true)) {
                    CabalInviteSheet(cabal: .sample(role: role), mode: mode)
                }
        }
    }
}
#endif
