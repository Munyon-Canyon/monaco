#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum CommentThreadScenario: String, CaseIterable {
    case logan
    case nvda
    case pilates
    case longThread
    case passed
    case failed

    static let launchArgument = "-commentThreadHarness"

    static func matching(_ arguments: [String]) -> CommentThreadScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return CommentThreadScenario(rawValue: arguments[flag + 1])
    }

    var permissions: CommentPermissions {
        guard self == .pilates else { return CommentPermissions(canComment: true) }
        return CommentPermissions(
            canComment: false, canReact: true, replyRule: .members(cabalName: "Pilates", cabalPictureURL: nil))
    }

    var threads: [Components.Schemas.CommentThread] {
        typealias Thread = Components.Schemas.CommentThread
        switch self {
        case .logan: return Thread.loganAAPLThread.filter { $0.comment.id != "aapl-jordan" }
        case .nvda: return Thread.nvdaThread
        case .pilates: return Thread.pilatesThread
        case .longThread:
            return (0..<30).map { index in
                Thread(
                    comment: .sample(
                        "long-\(index)", author: index.isMultiple(of: 2) ? "Maya" : "Alex",
                        body: "Reply number \(index + 1) in a long thread.", minutesAgo: Double(300 - index * 5)),
                    replies: [])
            }
        case .passed: return [Thread.nvdaPassedMonacoReply] + Thread.nvdaThread
        case .failed:
            return [
                Thread.tslaFailedMonacoReply,
                Thread(
                    comment: .sample(
                        "tsla-logan", author: "Logan", body: "Should have taken the win.", minutesAgo: 600,
                        reactions: [.init(emoji: "❤️", count: 1, mine: false)]),
                    replies: []),
            ]
        }
    }
}

private struct SampleCommentsSource: CommentsSource {
    let threads: [Components.Schemas.CommentThread]
    var permissions = CommentPermissions(canComment: true)

    func list(cursor _: String?) async throws -> CommentsPage {
        CommentsPage(threads: threads, nextCursor: nil)
    }

    func post(
        body _: String?, attachment _: Components.Schemas.CommentAttachmentInput?, parentId _: String?,
        submission _: IdempotentSubmission
    ) async throws -> Components.Schemas.Comment {
        throw URLError(.notConnectedToInternet)
    }

    func delete(id _: String, submission _: IdempotentSubmission) async throws {}

    func permissions() async throws -> CommentPermissions { permissions }
}

private struct CommentThreadHarnessScreen: View {
    let scenario: CommentThreadScenario

    @State private var model: CommentsModel
    @State private var toasts = ToastCenter()

    init(scenario: CommentThreadScenario) {
        self.scenario = scenario
        let threads = scenario.threads
        let reactions = Dictionary(
            uniqueKeysWithValues: threads.flatMap { [$0.comment] + $0.replies }.map { ($0.id, $0.reactions) })
        _model = State(
            initialValue: CommentsModel(
                source: SampleCommentsSource(threads: threads, permissions: scenario.permissions),
                hints: SampleSilentHints(), clock: ContinuousClock(),
                reactions: ReactionsModel.sample(
                    reactions: reactions, fails: false, clock: ContinuousClock())))
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                CommentThreadView(model: model)
                    .padding(.vertical, MonacoTheme.Space.s)
            }
            .environment(\.sectionScrollProxy, proxy)
        }
        .safeAreaInset(edge: .bottom, spacing: 0) {
            if scenario == .pilates { CommentComposerBar(model: model) }
        }
        .monacoCanvas()
        .environment(toasts)
        .monacoToastCenter(toasts)
        .task { await model.load() }
    }
}

final class CommentThreadSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let scenario = CommentThreadScenario.matching(arguments) else { return nil }
        return AnyView(NavigationStack { CommentThreadHarnessScreen(scenario: scenario) })
    }
}
#endif
