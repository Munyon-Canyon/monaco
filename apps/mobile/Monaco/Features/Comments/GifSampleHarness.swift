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
    case failedTile
    case live
    case bubble
    case reduceMotion
    case viewer

    var query: String {
        switch self {
        case .search: "dance"
        case .empty: "zzzz"
        case .trending, .failed, .failedTile, .live, .bubble, .reduceMotion, .viewer: ""
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
        case .failedTile: GifFailedTileProvider()
        case .empty: SampleGifProvider(mode: .noMatches)
        case .failed: SampleGifProvider(mode: .failing)
        case .live: AppEnvironment.makeGifProvider(key: Config.giphySDKKey, isRunningTests: false)
        }
    }
}

struct GifFailedTileProvider: GifProvider {
    static let brokenStill = URL(string: "https://gif-failure.invalid/still.png")

    private let base = SampleGifProvider()

    func trending(offset: Int) async throws -> GifPage {
        breaking(try await base.trending(offset: offset))
    }

    func search(_ query: String, offset: Int) async throws -> GifPage {
        breaking(try await base.search(query, offset: offset))
    }

    private func breaking(_ page: GifPage) -> GifPage {
        guard let broken = Self.brokenStill, let first = page.items.first else { return page }
        let gif = Gif(
            id: first.id, title: first.title,
            still: GifMedia(url: broken, width: first.still.width, height: first.still.height),
            preview: GifMedia(url: broken, width: first.preview.width, height: first.preview.height),
            original: first.original)
        return GifPage(
            items: [gif] + page.items.dropFirst(), nextOffset: page.nextOffset, attribution: page.attribution)
    }
}

enum GifHost: String {
    case sheet
    case docked
    case reply
}

struct GifSampleStill: View {
    let id: String

    nonisolated static func scene(for id: String) -> MonacoWorldScene {
        guard let number = id.split(separator: "-").last.flatMap({ Int($0) }) else {
            return MonacoWorldScene.forSeed(id)
        }
        return MonacoWorldScene(index: (number * 3) % MonacoWorldScene.count)
    }

    var body: some View {
        Color.clear.overlay {
            Image(Self.scene(for: id).imageName)
                .resizable()
                .scaledToFill()
        }
        .clipped()
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
            .toolbar(.hidden, for: .tabBar)
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
        client?.urlProtocol(self, didLoad: SampleGifData.bytes(for: url))
        client?.urlProtocolDidFinishLoading(self)
    }

    nonisolated override func stopLoading() {}
}

nonisolated enum SampleGifData {
    private static let zooms: [CGFloat] = [1.0, 1.06, 1.12, 1.06]

    static func bytes(for url: URL) -> Data {
        let id = url.pathComponents.dropFirst().first ?? ""
        guard let photo = UIImage(named: GifSampleStill.scene(for: id).imageName) else { return Data() }
        let data = NSMutableData()
        guard
            let destination = CGImageDestinationCreateWithData(
                data, UTType.gif.identifier as CFString, zooms.count, nil)
        else { return Data() }
        let file = [kCGImagePropertyGIFDictionary: [kCGImagePropertyGIFLoopCount: 0]]
        CGImageDestinationSetProperties(destination, file as CFDictionary)
        let frame = [kCGImagePropertyGIFDictionary: [kCGImagePropertyGIFDelayTime: 0.3]]
        let size = CGSize(width: 200, height: 150)
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        let renderer = UIGraphicsImageRenderer(size: size, format: format)
        for zoom in zooms {
            let image = renderer.image { _ in
                let fill = max(size.width / photo.size.width, size.height / photo.size.height) * zoom
                let drawn = CGSize(width: photo.size.width * fill, height: photo.size.height * fill)
                photo.draw(
                    in: CGRect(
                        x: (size.width - drawn.width) / 2, y: (size.height - drawn.height) / 2,
                        width: drawn.width, height: drawn.height))
            }
            if let cgImage = image.cgImage {
                CGImageDestinationAddImage(destination, cgImage, frame as CFDictionary)
            }
        }
        CGImageDestinationFinalize(destination)
        return data as Data
    }
}
#endif
