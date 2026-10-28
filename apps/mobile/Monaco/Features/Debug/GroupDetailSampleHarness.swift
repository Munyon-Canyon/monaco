#if DEBUG
import MonacoAPI
import MonacoCore
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
    case infoSaving
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
        case .infoAdmin, .infoSaving:
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
        case .info, .infoAdmin: return [CabalInfoRoute(cabalID: id)]
        case .infoSaving: return [CabalSampleRoute(kind: .infoSaving)]
        case .members: return [MembersRoute(cabalID: id, tab: .members)]
        case .voters: return [MembersRoute(cabalID: id, tab: .voters)]
        case .requests, .requestsEmpty: return [MembersRoute(cabalID: id, tab: .requests)]
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

nonisolated struct CabalSampleRoute: AppRoute {
    enum Kind: Hashable, Sendable {
        case publicMember
        case publicPrivate
        case infoSaving
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
        case .infoSaving:
            CabalInfoView(cabalID: cabalID, savingPreview: true)
        case .invite(let mode, let role):
            CabalScreen(cabalID: cabalID)
                .sheet(isPresented: .constant(true)) {
                    CabalInviteSheet(cabal: .sample(role: role), mode: mode)
                }
        }
    }
}
#endif
