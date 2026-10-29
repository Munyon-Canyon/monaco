#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

final class GifSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-gifHarness") else { return nil }
        let raw = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "trending"
        let scenario = GifScenario(rawValue: raw) ?? .trending
        let host =
            arguments.firstIndex(of: "-gifHost").flatMap { index in
                arguments.indices.contains(index + 1) ? GifHost(rawValue: arguments[index + 1]) : nil
            } ?? .sheet
        return AnyView(GifHarnessScreen(scenario: scenario, host: host, auth: auth))
    }
}

enum GifScenario: String {
    case trending
    case search
    case empty
    case failed
    case live

    var query: String {
        switch self {
        case .search: "dance"
        case .empty: "zzzz"
        case .trending, .failed, .live: ""
        }
    }

    @MainActor
    var provider: any GifProvider {
        switch self {
        case .trending, .search: SampleGifProvider()
        case .empty: SampleGifProvider(mode: .noMatches)
        case .failed: SampleGifProvider(mode: .failing)
        case .live: AppEnvironment.makeGifProvider(key: Config.giphySDKKey, isRunningTests: false)
        }
    }
}

enum GifHost: String {
    case sheet
    case docked
    case reply
}

struct GifSampleStill: View {
    let id: String

    private static let palette: [Color] = [
        MonacoTheme.brand, MonacoTheme.profit, MonacoTheme.warning, MonacoTheme.loss,
    ]

    var body: some View {
        let index = id.unicodeScalars.reduce(0) { $0 + Int($1.value) } % Self.palette.count
        Self.palette[index].opacity(0.55)
    }
}

private struct GifRecordingSource: CommentsSource {
    let onPost: @MainActor @Sendable (Components.Schemas.CommentAttachmentInput?) -> Void

    func list(cursor _: String?) async throws -> CommentsPage {
        let thread = Components.Schemas.CommentThread(
            comment: .sample("c1", author: "Mia", body: "What would our NVIDIA allocation be after this?"),
            replies: [])
        return CommentsPage(threads: [thread], nextCursor: nil)
    }

    func post(
        body _: String?, attachment: Components.Schemas.CommentAttachmentInput?, parentId _: String?,
        submission _: IdempotentSubmission
    ) async throws -> Components.Schemas.Comment {
        await onPost(attachment)
        return .sample("c9", mine: true)
    }

    func delete(id _: String, submission _: IdempotentSubmission) async throws {}

    func canComment() async throws -> Bool { true }
}

private struct GifHarnessScreen: View {
    let scenario: GifScenario
    let host: GifHost
    let auth: PrivyAuthService

    @State private var environment: AppEnvironment?
    @State private var comments: CommentsModel?
    @State private var toasts = ToastCenter()
    @State private var presents: Bool
    @State private var postCount = 0
    @State private var lastPost: Components.Schemas.CommentAttachmentInput?

    init(scenario: GifScenario, host: GifHost, auth: PrivyAuthService) {
        self.scenario = scenario
        self.host = host
        self.auth = auth
        _presents = State(initialValue: host == .sheet)
    }

    var body: some View {
        Color.clear
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
            .overlay(alignment: .top) { postsLabel }
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if let comments, host != .sheet { CommentComposerBar(model: comments) }
            }
            .sheet(isPresented: $presents) {
                GifSheet(gifs: scenario.provider, query: scenario.query) { attachment in
                    record(attachment)
                }
            }
            .environment(toasts)
            .monacoToastCenter(toasts)
            .optionalEnvironment(environment)
            .task { await prepare() }
    }

    private var postsLabel: some View {
        Text(summary)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .padding(.top, MonacoTheme.Space.l)
            .accessibilityIdentifier("gif-harness-posts")
    }

    private func prepare() async {
        environment = AppEnvironment(
            auth: auth,
            tokens: SessionTokens(privyToken: { "sample-token" }, refresh: { _ in nil }),
            hints: SampleSilentHints(),
            gifs: scenario.provider,
            defaults: UserDefaults(suiteName: "MonacoGifHarness") ?? .standard,
            isAuthenticated: { true },
            endAuthSession: {}
        )
        let model = CommentsModel(
            source: GifRecordingSource { attachment in record(attachment) },
            hints: SampleSilentHints(), clock: ContinuousClock())
        comments = model
        await model.load()
        if host == .reply, let row = model.rows.first { model.presentReplySheet(.comment(row)) }
    }

    private var summary: String {
        guard case .gif(let input) = lastPost else { return "posts=\(postCount)" }
        return "posts=\(postCount) kind=\(input.kind.rawValue) id=\(input.gif.id)"
    }

    private func record(_ attachment: Components.Schemas.CommentAttachmentInput?) {
        postCount += 1
        lastPost = attachment
    }
}

extension View {
    @ViewBuilder fileprivate func optionalEnvironment(_ environment: AppEnvironment?) -> some View {
        if let environment {
            self.environment(environment)
        } else {
            self
        }
    }
}
#endif
