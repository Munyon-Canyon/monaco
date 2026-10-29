#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

/// Debug-only sample data for the Cabals tab. Launch with
/// `-MonacoCabalsTabSample` to open the tab without sign-in or a backend
/// (screenshots and XCUITests). Never compiled into Release builds.
enum CabalsTabSampleData {
    static let launchArgument = "-MonacoCabalsTabSample"
    static let scenarioArgument = "-MonacoCabalsTabSampleScenario"

    /// What the tab is booted into. Add a scenario rather than a second harness:
    /// the point of this file is that every Cabals screenshot comes from the
    /// same fixed cabals.
    enum Scenario: String {
        /// Signed in, cabals loaded. The default.
        case normal
        /// The cabals list, their return chart and the board all failed to read.
        /// Each offers a retry and none claims the member has no cabals.
        case cabalsUnavailable
        /// No read has answered yet, so the list, the chart and the board show
        /// their placeholders.
        case cabalsLoading
        case ready
        case noAction
        case empty
        case filtered
        case noShare
        case pnlUnavailable
        case rankLoading
        case rankError
        case rankEmpty
        case fReady
        case fUnfunded
        case fMenu
    }

    static func matches(_ arguments: [String]) -> Bool {
        arguments.contains(launchArgument)
    }

    static var scenario: Scenario {
        let arguments = ProcessInfo.processInfo.arguments
        guard let flag = arguments.firstIndex(of: scenarioArgument),
            arguments.indices.contains(flag + 1),
            let scenario = Scenario(rawValue: arguments[flag + 1])
        else { return .normal }
        return scenario
    }

    @MainActor
    static var script: SampleAPIScript {
        switch scenario {
        case .normal: SampleAPIScript()
        case .cabalsUnavailable: SampleAPIScript(mode: .cabalsUnavailable)
        case .cabalsLoading: SampleAPIScript(mode: .hang)
        case .ready, .filtered: HomeSampleData.script(.loss, proposals: [.needsVote])
        case .noAction: HomeSampleData.script(.loss, proposals: [])
        case .empty: emptyScript
        case .noShare: unfundedScript
        case .pnlUnavailable: noReturnScript
        case .rankLoading: ready(board: .hanging)
        case .rankError: ready(board: .failing)
        case .rankEmpty: ready(board: .empty)
        case .fReady, .fMenu: fReadyScript
        case .fUnfunded: fUnfundedScript
        }
    }

    @MainActor
    private static var fReadyScript: SampleAPIScript {
        var script = HomeSampleData.script(.loss, proposals: [.fVote])
        script.portfolio = .sampleOwnProfile
        script.pendingVotes = script.pendingVotes.map { vote in
            var moved = vote
            moved.cabalId = Components.Schemas.CabalRef.sampleACCapital.id
            moved.cabalName = Components.Schemas.CabalRef.sampleACCapital.name
            return moved
        }
        script.invites = [quantKidsInvite]
        return script
    }

    @MainActor
    private static var fUnfundedScript: SampleAPIScript {
        var script = HomeSampleData.script(.loss, proposals: [])
        var portfolio = Components.Schemas.MyPortfolio.sampleLoss
        portfolio.cabals = Array(portfolio.cabals.suffix(1))
        script.portfolio = portfolio
        return script
    }

    private static var quantKidsInvite: Components.Schemas.CabalInvite {
        var invite = Components.Schemas.CabalInvite.sample
        invite.cabal = Components.Schemas.CabalInviteCabal(
            id: Components.Schemas.CabalRef.sampleQuantKids.id, name: Components.Schemas.CabalRef.sampleQuantKids.name,
            pictureUrl: nil, memberCount: 5)
        return invite
    }

    @MainActor
    private static var emptyScript: SampleAPIScript {
        var script = HomeSampleData.script(.empty, proposals: [])
        script.mode = .empty
        return script
    }

    @MainActor
    private static var unfundedScript: SampleAPIScript {
        var script = HomeSampleData.script(.loss, proposals: [.needsVote])
        script.portfolio?.cabals[0].valueMicros = 0
        script.portfolio?.cabals[0].pnlMicros = 0
        script.portfolio?.cabals[0].returnBps = nil
        script.portfolio?.cabals[0].sliceBps = 0
        script.ownershipBps[HomeSampleData.bigBallers] = 0
        return script
    }

    @MainActor
    private static var noReturnScript: SampleAPIScript {
        var script = HomeSampleData.script(.loss, proposals: [.needsVote])
        script.portfolio?.cabals[0].returnBps = nil
        return script
    }

    @MainActor
    private static func ready(board: SampleAPIScript.CabalBoardMode) -> SampleAPIScript {
        var script = HomeSampleData.script(.loss, proposals: [.needsVote])
        script.cabalBoardMode = board
        return script
    }

    nonisolated static let createdCabalID = "5b1f0c9e-0007-4c55-9a51-000000000007"

    /// A stubbed create that always succeeds.
    @MainActor
    struct Actions: CabalsActionSource {
        func createCabal(_ input: CreateCabalInput, submission: IdempotentSubmission) async throws
            -> Components.Schemas.Cabal
        {
            try await Task.sleep(for: .milliseconds(120))
            var created = Components.Schemas.Cabal.sample(role: "creator")
            created.id = createdCabalID
            created.name = input.name
            return created
        }
    }
}

struct CabalsTabSampleHarness: View {
    @ObservedObject var auth: PrivyAuthService

    var body: some View {
        SampleAppFrame(
            auth: auth, tab: .cabals,
            session: CabalsTabSampleData.scenario == .cabalsLoading ? SampleAppFrame.loading : SampleAppFrame.signedIn)
    }
}

final class CabalsTabSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard CabalsTabSampleData.matches(arguments) else { return nil }
        SampleAPIProtocol.install(CabalsTabSampleData.script)
        return AnyView(CabalsTabSampleHarness(auth: auth))
    }
}
extension LeaderboardFilters {
    static var cabalsTabSample: LeaderboardFilters {
        guard CabalsTabSampleData.matches(ProcessInfo.processInfo.arguments), CabalsTabSampleData.scenario == .filtered
        else { return .standard }
        var filters = LeaderboardFilters.standard
        filters.sort = .pnlAmount
        return filters
    }
}

final class CreateCabalSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-MonacoCreateCabalSample"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: launchArgument) else { return nil }
        let state = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "ready"
        let tooLong = String(repeating: "Sunday ", count: 6)
        let form: CreateCabalForm
        switch state {
        case "invalid": form = CreateCabalForm(name: tooLong)
        case "selectedVoters": form = CreateCabalForm(name: "Sunday Investors", voterMode: .picked)
        default: form = CreateCabalForm(name: "Sunday Investors", joinPolicy: .request, expiry: .oneDay)
        }
        return AnyView(
            NavigationStack {
                CreateGroupView(
                    actions: CabalsTabSampleData.Actions(), form: form, isCreating: state == "creating",
                    submitFailed: state == "error", autofocus: false)
            }
        )
    }
}
#endif
