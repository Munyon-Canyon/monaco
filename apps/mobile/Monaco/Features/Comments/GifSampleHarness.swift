#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit
import UniformTypeIdentifiers

final class GifSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-gifHarness") else { return nil }
        let raw = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "trending"
        let scenario = GifScenario(rawValue: raw) ?? .trending
        let host =
            arguments.firstIndex(of: "-gifHost").flatMap { index in
                arguments.indices.contains(index + 1) ? GifHost(rawValue: arguments[index + 1]) : nil
            } ?? (scenario.showsThread ? .docked : .sheet)
        return AnyView(GifHarnessScreen(scenario: scenario, host: host, auth: auth))
    }
}

enum GifScenario: String {
    case trending
    case search
    case empty
    case failed
    case live
    case bubble
    case reduceMotion
    case viewer

    var query: String {
        switch self {
        case .search: "dance"
        case .empty: "zzzz"
        case .trending, .failed, .live, .bubble, .reduceMotion, .viewer: ""
        }
    }

    var showsThread: Bool {
        self == .bubble || self == .reduceMotion
    }

    var drawsProposal: Bool { self != .reduceMotion }

    @MainActor
    var provider: any GifProvider {
        switch self {
        case .trending, .search, .bubble, .reduceMotion, .viewer: SampleGifProvider()
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

extension Components.Schemas.GifMedia {
    fileprivate nonisolated init(_ media: GifMedia) {
        self.init(url: media.url.absoluteString, width: media.width, height: media.height)
    }
}

private actor GifHarnessReplies {
    private var posted: [Components.Schemas.Comment] = []

    func add(_ comment: Components.Schemas.Comment) {
        posted.append(comment)
    }

    func all() -> [Components.Schemas.Comment] {
        posted
    }
}

private struct GifRecordingSource: CommentsSource {
    let seeded: Bool
    let onPost: @MainActor @Sendable (Components.Schemas.CommentAttachmentInput?) -> Void
    private let replies = GifHarnessReplies()

    private static var seededReply: Components.Schemas.Comment {
        var comment = Components.Schemas.Comment.gifSample(
            "g1", author: "Jordan", minutesAgo: 0,
            quote: .init(
                commentId: "c1", rootCommentId: "c1", authorDisplayName: "Mia",
                excerpt: "we can add on a dip", isDeleted: false, hasAttachment: false, isFolded: false),
            reactions: [.init(emoji: "❤️", count: 2, mine: false)])
        if let gif = SampleGifProvider.items.dropFirst().first {
            comment.attachment = .gif(
                .init(
                    kind: .gif, id: gif.id, title: gif.title, still: .init(gif.still), preview: .init(gif.preview),
                    original: .init(gif.original)))
        }
        return comment
    }

    init(
        seeded: Bool, onPost: @escaping @MainActor @Sendable (Components.Schemas.CommentAttachmentInput?) -> Void
    ) {
        self.seeded = seeded
        self.onPost = onPost
    }

    func list(cursor _: String?) async throws -> CommentsPage {
        let thread = Components.Schemas.CommentThread(
            comment: .sample("c1", author: "Mia", body: "we can add on a dip"), replies: [])
        let root = seeded ? Components.Schemas.CommentThread(comment: Self.seededReply, replies: []) : thread
        let posted = await replies.all().map { Components.Schemas.CommentThread(comment: $0, replies: []) }
        return CommentsPage(threads: [root] + posted, nextCursor: nil)
    }

    func post(
        body _: String?, attachment: Components.Schemas.CommentAttachmentInput?, parentId _: String?,
        submission _: IdempotentSubmission
    ) async throws -> Components.Schemas.Comment {
        await onPost(attachment)
        var comment = Components.Schemas.Comment.sample("c9", author: "Jordan", mine: true)
        if case .gif(let input)? = attachment {
            comment.body = nil
            comment.bodyDisplay = "GIF"
            comment.attachment = .gif(
                .init(
                    kind: .gif, id: input.gif.id, title: input.gif.title, still: input.gif.still,
                    preview: input.gif.preview, original: input.gif.original))
            await replies.add(comment)
        }
        return comment
    }

    func delete(id _: String, submission _: IdempotentSubmission) async throws {}

    func permissions() async throws -> CommentPermissions { CommentPermissions(canComment: true) }
}

nonisolated struct GifProposalSampleRoute: AppRoute {
    let scrolled: Bool

    @MainActor func destination() -> some View {
        GifProposalSamplePage(scrolled: scrolled)
    }
}

private struct GifProposalSamplePage: View {
    let scrolled: Bool
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.proposalComments) private var comments
    @State private var detail = ProposalDetailBox()

    var body: some View {
        let model = detail.model(proposalID: "proposal-1", environment: environment)
        let sections: [any ProposalSection.Type] = [ProposalSampleDetailSlot.self, ProposalCommentsSlot.self]
        SectionStack(context: ProposalContext(proposalID: "proposal-1"), sections: sections.map { $0.erased })
            .defaultScrollAnchor(scrolled ? .bottom : .top)
            .environment(\.proposalDetail, model)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    MonacoGlassButton(icon: .share, accessibilityLabel: "Share", style: .quiet, placement: .toolbar) {}
                }
            }
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if let comments {
                    CommentComposerBar(
                        model: comments, thesis: model.value?.summary.thesis, offersPoll: true, asksCabal: true)
                }
            }
    }
}

private struct GifHarnessScreen: View {
    let scenario: GifScenario
    let host: GifHost
    let auth: PrivyAuthService

    @State private var environment: AppEnvironment?
    @State private var comments: CommentsModel?
    @State private var toasts = ToastCenter()
    @State private var presents: Bool
    @State private var viewing: Bool
    @State private var postCount = 0
    @State private var lastPost: Components.Schemas.CommentAttachmentInput?

    init(scenario: GifScenario, host: GifHost, auth: PrivyAuthService) {
        self.scenario = scenario
        self.host = host
        self.auth = auth
        if scenario.drawsProposal { SampleAPIProtocol.install(ProposalSampleScenario.nvdaOpen.script) }
        _presents = State(initialValue: host == .sheet && scenario != .viewer)
        _viewing = State(initialValue: scenario == .viewer)
    }

    var body: some View {
        thread
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
            .overlay(alignment: .top) { postsLabel }
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if let comments, host != .sheet, !scenario.drawsProposal { CommentComposerBar(model: comments) }
            }
            .sheet(isPresented: $presents) {
                GifSheet(gifs: scenario.provider, query: scenario.query) { attachment in
                    record(attachment)
                }
            }
            .fullScreenCover(isPresented: $viewing) {
                if let item = SampleGifProvider.items.first {
                    MediaViewer(gif: item.original.url, still: item.still.url)
                }
            }
            .environment(\.gifStillsOnly, scenario == .reduceMotion)
            .environment(toasts)
            .monacoToastCenter(toasts)
            .optionalEnvironment(environment)
            .task { await prepare() }
    }

    @ViewBuilder private var thread: some View {
        if scenario.drawsProposal {
            SampleAppFrame(auth: auth, tab: .cabals, routes: [GifProposalSampleRoute(scrolled: scenario == .bubble)])
                .environment(\.proposalComments, comments)
        } else if let comments, scenario.showsThread {
            ScrollView {
                CommentThreadView(model: comments)
                    .padding(.top, MonacoTheme.Space.xl)
            }
        } else {
            Color.clear
        }
    }

    private var postsLabel: some View {
        Color.clear
            .frame(width: 1, height: 1)
            .accessibilityElement()
            .accessibilityLabel(summary)
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
            source: GifRecordingSource(seeded: scenario.showsThread) { attachment in record(attachment) },
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

nonisolated final class SampleGifURLProtocol: URLProtocol {
    nonisolated override class func canInit(with request: URLRequest) -> Bool {
        request.url?.host == SampleGifProvider.host
    }

    nonisolated override class func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    nonisolated override func startLoading() {
        guard let url = request.url,
            let response = HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil, headerFields: nil)
        else { return }
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: SampleGifData.bytes)
        client?.urlProtocolDidFinishLoading(self)
    }

    nonisolated override func stopLoading() {}
}

nonisolated enum SampleGifData {
    static let bytes: Data = {
        let colors: [UIColor] = [.systemBlue, .systemGreen, .systemOrange, .systemRed]
        let data = NSMutableData()
        guard
            let destination = CGImageDestinationCreateWithData(
                data, UTType.gif.identifier as CFString, colors.count, nil)
        else { return Data() }
        let file = [kCGImagePropertyGIFDictionary: [kCGImagePropertyGIFLoopCount: 0]]
        CGImageDestinationSetProperties(destination, file as CFDictionary)
        let frame = [kCGImagePropertyGIFDictionary: [kCGImagePropertyGIFDelayTime: 0.3]]
        let size = CGSize(width: 200, height: 150)
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        let renderer = UIGraphicsImageRenderer(size: size, format: format)
        for (index, color) in colors.enumerated() {
            let image = renderer.image { context in
                color.setFill()
                context.fill(CGRect(origin: .zero, size: size))
                UIColor.white.setFill()
                context.fill(CGRect(x: CGFloat(index) * 50, y: 50, width: 50, height: 50))
            }
            if let cgImage = image.cgImage {
                CGImageDestinationAddImage(destination, cgImage, frame as CFDictionary)
            }
        }
        CGImageDestinationFinalize(destination)
        return data as Data
    }()
}
#endif
