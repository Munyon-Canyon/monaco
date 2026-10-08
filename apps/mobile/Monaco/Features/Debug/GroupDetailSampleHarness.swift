#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum GroupDetailSampleScenario: String, CaseIterable {
    case populated
    case empty
    case loading
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

extension GroupDetailSampleScenario {
    @MainActor
    var script: SampleAPIScript {
        switch self {
        case .populated: SampleAPIScript(role: "member")
        case .empty: SampleAPIScript(mode: .empty, role: "creator")
        case .loading: SampleAPIScript(mode: .hang, role: "member")
        case .picture: SampleAPIScript(role: "creator", pictureURL: GroupDetailSampleData.pictureURL)
        case .noPicture: SampleAPIScript(role: "creator")
        case .pictureNotCreator: SampleAPIScript(role: "member", pictureURL: GroupDetailSampleData.pictureURL)
        case .pictureUploadFailure:
            SampleAPIScript(role: "creator", pictureURL: GroupDetailSampleData.pictureURL, pictureWriteFails: true)
        }
    }
}

enum GroupDetailSampleData {
    static let cabalID = Components.Schemas.Cabal.sample(role: nil).id

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
            auth: auth, tab: .cabals, routes: [CabalRoute(id: GroupDetailSampleData.cabalID)],
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
#endif
