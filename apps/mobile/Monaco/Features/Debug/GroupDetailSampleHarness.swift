#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

/// Debug-only: the group screen and its pushed screens on canned data, no sign-in or backend.
/// Launch with `-MonacoGroupDetailSample <scenario>`:
/// `populated` · `empty` · `loading` · `details` (Cabal details sheet open) · `propose` (chooser sheet open)
/// · `picture` (cabal with a picture, viewer is its creator) · `noPicture` (creator, tinted
/// initials, nothing to remove) · `pictureNotCreator` (has a picture, viewer is a plain member,
/// so no controls) · `pictureUploadFailure` (every write is refused).
enum GroupDetailSampleScenario: String, CaseIterable {
    case populated
    case empty
    case loading
    case details
    case propose
    case picture
    case noPicture
    case pictureNotCreator
    case pictureUploadFailure

    static let launchArgument = "-MonacoGroupDetailSample"

    static func matching(_ arguments: [String]) -> GroupDetailSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return GroupDetailSampleScenario(rawValue: arguments[flag + 1])
    }
}

struct GroupDetailSampleHarness: View {
    let scenario: GroupDetailSampleScenario
    @ObservedObject var auth: PrivyAuthService

    @State private var session: AppSessionStore = {
        let session = AppSessionStore(apiClient: MonacoAPIClient())
        session.isLoading = false
        session.profile = ProfileSampleHarness.sampleProfile(
            userID: GroupDetailSampleData.viewerId,
            displayName: "Logan Norman",
            photoURL: nil
        )
        return session
    }()
    @State private var proposalService = SampleProposalFeedService()
    @State private var showDetails = false
    @State private var showPropose = false
    @State private var route: GroupDetailRoute?
    @State private var toast: MonacoToast?
    @State private var heroScrolledAway = false
    /// Every scenario gets an editor; only the ones whose sample view says the
    /// viewer is the creator actually show its controls.
    @StateObject private var pictureEditor: CabalPictureEditor

    init(scenario: GroupDetailSampleScenario, auth: PrivyAuthService) {
        self.scenario = scenario
        self.auth = auth
        _pictureEditor = StateObject(
            wrappedValue: CabalPictureEditor(
                groupId: GroupDetailSampleData.pictureGroupId,
                pictureUrl: GroupDetailSampleData.initialPictureURL(for: scenario),
                writer: SampleCabalPictureWriter(alwaysFails: scenario == .pictureUploadFailure)
            ))
    }

    var body: some View {
        NavigationStack {
            root
        }
        .tint(MonacoTheme.ink)
        .environment(session)
    }

    @ViewBuilder
    private var root: some View {
        switch scenario {
        case .loading:
            GroupDetailSkeleton()
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                .monacoCanvas()
                .navigationTitle("Weekend investors")
                .navigationBarTitleDisplayMode(.inline)
        case .populated, .empty, .details, .propose:
            groupScreen(scenario == .empty ? GroupDetailSampleData.emptyView : GroupDetailSampleData.view)
        case .picture, .noPicture, .pictureNotCreator, .pictureUploadFailure:
            groupScreen(GroupDetailSampleData.pictureView(for: scenario))
        }
    }

    /// Same content and chrome as `GroupDetailView`, fed from sample data.
    private func groupScreen(_ view: GroupViewDTO) -> some View {
        GroupDetailContent(
            auth: auth,
            view: view,
            currentUserId: GroupDetailSampleData.viewerId,
            proposalService: proposalService,
            proposalRefreshToken: "0",
            onRoute: { route = $0 },
            onPropose: { showPropose = true },
            onToast: { toast = $0 },
            onHeroScrolledAway: { heroScrolledAway = $0 },
            pictureEditor: pictureEditor,
            heroChart: scenario == .empty ? .sparse : .curve(GroupDetailSampleData.pnlPoints),
            heroRange: .oneMonth
        )

        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoCanvas()
        .navigationTitle(heroScrolledAway ? view.name : "")
        .navigationBarTitleDisplayMode(.inline)
        .cabalHeroNavigationBar(isOverHero: !heroScrolledAway)

        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    showDetails = true
                } label: {
                    Image(systemName: "info.circle")
                }
                .accessibilityLabel("Cabal details")
                .accessibilityIdentifier("group-details-button")
            }
        }
        .navigationDestination(item: $route) { route in
            switch route {
            case .proposals:
                ProposalFeedView(service: proposalService, groupId: view.id)
            case .stock(let symbol):
                AssetDetailClientView(symbol: symbol)
            case .chat:
                Text("Not in the sample harness")
                    .foregroundStyle(MonacoTheme.muted)
            }
        }
        .sheet(isPresented: $showPropose) {
            ProposeSheet(auth: auth, groupId: view.id, groupView: view)
        }
        .sheet(isPresented: $showDetails) {
            GroupDetailsSheet(treasuryAddress: view.treasuryAddress)
        }
        .monacoToast($toast)
        .task {
            if scenario == .details { showDetails = true }
            if scenario == .propose { showPropose = true }
        }
    }
}

enum GroupDetailSampleData {
    static let viewerId = "u2"

    /// A month of the pot's P&L, three days a sample, ending on the hero's all-time figure.
    static var pnlPoints: [GroupPnLPointDTO] {
        let path: [Double] = [0, 4.1, 9.8, 7.2, 15.5, 12.0, 19.4, 24.7, 18.9, 20.87]
        let step: TimeInterval = 3 * 24 * 3600
        let now = Date()
        return path.enumerated().map { index, value in
            GroupPnLPointDTO(
                at: now.addingTimeInterval(-step * Double(path.count - 1 - index)),
                potValueUsd: "548.20",
                netInUsd: "527.33",
                dollarPnl: String(format: "%+.2f", value)
            )
        }
    }

    static let view = GroupViewDTO(
        id: "5b1f0c9e-0001-4c55-9a51-000000000001",
        name: "Weekend investors",
        treasuryAddress: "9fQeWbKc3pZ1rJtM7xVn2aLhYd8sGu4oTk6ReXbN1cPq",
        potTotalUsd: "548.20",
        pot: [
            PotRowDTO(
                symbol: "USDC", units: "82.62", markUsd: "1.00", valueUsd: "82.62", dollarPnl: "+0.00", afterHours: nil,
                tokenAmount: nil),
            PotRowDTO(
                symbol: "AAPLx", units: "1.2034", markUsd: "231.40", valueUsd: "278.47", dollarPnl: "+28.47",
                afterHours: false, tokenAmount: "120340000"),
            PotRowDTO(
                symbol: "NVDAx", units: "1.05", markUsd: "178.20", valueUsd: "187.11", dollarPnl: "-7.60",
                afterHours: true, tokenAmount: "105000000"),
        ],
        you: MemberSliceDTO(
            shareUnits: "311500000", equityUsd: "311.50", slicePercent: "0.568", dollarPnl: "+27.40",
            percentReturn: "0.096"),
        members: [
            LeaderboardRowDTO(
                rank: 1, userId: "u1", displayName: "Ana Ruiz", percentReturn: "0.142", dollarPnl: "+14.20"),
            LeaderboardRowDTO(
                rank: 2, userId: "u2", displayName: "Logan Norman", percentReturn: "0.096", dollarPnl: "+27.40"),
            LeaderboardRowDTO(
                rank: 3, userId: "u3", displayName: "Leo Park", percentReturn: "0.012", dollarPnl: "+1.10"),
            LeaderboardRowDTO(
                rank: 4, userId: "u4", displayName: "Mia Chen", percentReturn: "-0.021", dollarPnl: "-2.10"),
            LeaderboardRowDTO(rank: 5, userId: "u5", displayName: "Sam Okafor", percentReturn: nil, dollarPnl: "+0.00"),
        ],
        proposals: nil,
        agent: GroupAgentDTO(
            id: "a1", status: "active", agentDisplayName: "Scout", allocationUsdcMicros: "100000000", apiKey: "scout")
    )

    static let emptyView = GroupViewDTO(
        id: "5b1f0c9e-0009-4c55-9a51-000000000009",
        name: "Rent money",
        treasuryAddress: "9fQeWbKc3pZ1rJtM7xVn2aLhYd8sGu4oTk6ReXbN1cPq",
        potTotalUsd: "0.00",
        pot: [
            PotRowDTO(
                symbol: "USDC", units: "0.00", markUsd: "1.00", valueUsd: "0.00", dollarPnl: "+0.00", afterHours: nil,
                tokenAmount: nil)
        ],
        you: MemberSliceDTO(
            shareUnits: "0", equityUsd: "0.00", slicePercent: "0", dollarPnl: "+0.00", percentReturn: nil),
        members: [
            LeaderboardRowDTO(
                rank: 1, userId: "u2", displayName: "Logan Norman", percentReturn: nil, dollarPnl: "+0.00")
        ],
        proposals: nil,
        agent: nil
    )

    // MARK: - Cabal picture scenarios

    static let pictureGroupId = "5b1f0c9e-0001-4c55-9a51-000000000001"

    /// The picture a scenario starts with. A file URL, so the mark draws a real
    /// image through the same image store with no network.
    static func initialPictureURL(for scenario: GroupDetailSampleScenario) -> String? {
        switch scenario {
        case .picture, .pictureNotCreator, .pictureUploadFailure:
            samplePictureURL()?.absoluteString
        default:
            nil
        }
    }

    /// The group view behind each picture scenario. Only the two creator
    /// scenarios say the viewer created the cabal, so the others must not offer
    /// the controls at all.
    static func pictureView(for scenario: GroupDetailSampleScenario) -> GroupViewDTO {
        let view = self.view
        return GroupViewDTO(
            id: view.id,
            name: view.name,
            treasuryAddress: view.treasuryAddress,
            potTotalUsd: view.potTotalUsd,
            pot: view.pot,
            you: view.you,
            members: view.members,
            proposals: view.proposals,
            agent: view.agent,
            pictureUrl: initialPictureURL(for: scenario),
            isCreator: scenario != .pictureNotCreator
        )
    }

    /// A generated square written to tmp, so the cabal mark shows a picture with
    /// no backend and no network.
    static func samplePictureURL() -> URL? {
        let size = CGSize(width: 256, height: 256)
        let image = UIGraphicsImageRenderer(size: size).image { context in
            let cg = context.cgContext
            let colors =
                [
                    UIColor(red: 0.16, green: 0.36, blue: 0.75, alpha: 1).cgColor,
                    UIColor(red: 0.52, green: 0.22, blue: 0.70, alpha: 1).cgColor,
                ] as CFArray
            if let gradient = CGGradient(colorsSpace: CGColorSpaceCreateDeviceRGB(), colors: colors, locations: [0, 1])
            {
                cg.drawLinearGradient(
                    gradient, start: .zero, end: CGPoint(x: 256, y: 256), options: [.drawsAfterEndLocation])
            }
            UIColor(white: 1, alpha: 0.85).setFill()
            cg.fillEllipse(in: CGRect(x: 78, y: 62, width: 100, height: 100))
            UIColor(white: 1, alpha: 0.55).setFill()
            cg.fill(CGRect(x: 48, y: 176, width: 160, height: 22))
        }
        guard let data = image.pngData() else { return nil }
        let url = FileManager.default.temporaryDirectory.appending(path: "monaco-sample-cabal-picture.png")
        do {
            try data.write(to: url, options: .atomic)
            return url
        } catch {
            return nil
        }
    }
}

/// Stands in for the backend in the picture scenarios: a short delay so the
/// spinner is visible, then either a new picture or the failure the
/// `pictureUploadFailure` scenario exists to show.
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
        return GroupDetailSampleData.samplePictureURL()?.absoluteString
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

final class GroupDetailSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = GroupDetailSampleScenario.matching(arguments) else { return nil }
        return AnyView(GroupDetailSampleHarness(scenario: scenario, auth: auth))
    }
}
#endif
