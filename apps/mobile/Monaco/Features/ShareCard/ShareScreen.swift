import MonacoAPI
import MonacoCore
import SwiftUI
import UniformTypeIdentifiers

enum ShareReplay {
    static let duration: TimeInterval = 2.4

    static func progress(from start: Date, at now: Date) -> Double {
        min(max(now.timeIntervalSince(start) / duration, 0), 1)
    }

    static func eased(_ progress: Double) -> Double {
        1 - pow(1 - min(max(progress, 0), 1), 3)
    }

    static func spec(_ spec: ShareCardSpec, at progress: Double) -> ShareCardSpec {
        guard progress < 1 else { return spec }
        let figure = spec.figure
        let counted = ShareCardSpec.Figure(
            main: countUp(figure.main, progress: progress),
            secondary: figure.secondary.map { countUp($0, progress: progress) },
            context: figure.context, tone: figure.tone)
        return ShareCardSpec(
            header: spec.header, chart: spec.chart, tickerLine: spec.tickerLine, figure: counted, stats: spec.stats,
            code: spec.code, replayTitle: spec.replayTitle, facts: spec.facts, spoken: spec.spoken)
    }

    static func countUp(_ text: String, progress: Double) -> String {
        guard text.first?.isLetter != true,
            let range = text.range(of: #"[0-9][0-9,]*(\.[0-9]+)?"#, options: .regularExpression),
            let value = Double(text[range].replacingOccurrences(of: ",", with: ""))
        else { return text }
        let digits = text[range].split(separator: ".", omittingEmptySubsequences: false).dropFirst().first?.count ?? 0
        let formatter = NumberFormatter()
        formatter.locale = Locale(identifier: "en_US")
        formatter.numberStyle = .decimal
        formatter.usesGroupingSeparator = text[range].contains(",")
        formatter.minimumFractionDigits = digits
        formatter.maximumFractionDigits = digits
        guard let counted = formatter.string(from: NSNumber(value: value * eased(progress))) else { return text }
        return text.replacingCharacters(in: range, with: counted)
    }
}

@MainActor
enum ShareActions {
    static let copiedMessage = "Image copied."

    static func payload(for spec: ShareCardSpec, url: URL?, image: UIImage) -> SharePayload {
        SharePayload(
            title: "\(spec.header.title) \u{00B7} \(spec.tickerLine.symbol)", url: url ?? ShareCardContent.fallbackURL,
            image: image)
    }

    static func copy(_ image: UIImage, to pasteboard: UIPasteboard = .general) -> Bool {
        guard let data = image.pngData() else { return false }
        pasteboard.setItems([[UTType.png.identifier: data]])
        return true
    }
}

struct ShareScreen: View {
    let subject: ShareSubject

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: ShareScreenModel?

    var body: some View {
        Group {
            if let model {
                content(model)
            } else {
                ShareSkeleton()
            }
        }
        .monacoCanvas()
        .navigationTitle(model?.title ?? ShareScreenModel.positionTitle)
        .navigationBarTitleDisplayMode(.inline)
        .task {
            guard model == nil else { return }
            let created = ShareScreenModel(subject: subject, api: environment.api) { Date() }
            model = created
            await created.load()
        }
    }

    private func content(_ model: ShareScreenModel) -> some View {
        ShareScreenContent(
            state: model.state,
            eyes: Binding(get: { model.eyes }, set: { model.eyes = $0 }),
            format: Binding(get: { model.format }, set: { model.format = $0 }),
            unusedEyes: model.isVote ? [.pnl, .trades] : [],
            share: { spec, image in
                SystemShare.present(ShareActions.payload(for: spec, url: model.shareURL, image: image))
            },
            copyImage: { image in
                guard ShareActions.copy(image) else { return }
                toasts.current = MonacoToast(message: ShareActions.copiedMessage, isSuccess: true)
            },
            retry: { Task { await model.load() } })
    }
}

struct ShareSkeleton: View {
    var body: some View {
        SkeletonBlock(height: nil, radius: MonacoTheme.Radius.shareCard)
            .aspectRatio(4.0 / 5.0, contentMode: .fit)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.s)
            .frame(maxHeight: .infinity, alignment: .top)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Loading")
            .accessibilityIdentifier("share-loading")
    }
}

struct ShareScreenContent: View {
    let state: LoadState<ShareCardSpec>
    @Binding var eyes: ShareEyes
    @Binding var format: ShareFormat
    var frozenProgress: Double?
    var unusedEyes: Set<ShareEye> = []
    let share: (ShareCardSpec, UIImage) -> Void
    let copyImage: (UIImage) -> Void
    let retry: () -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var replayStart: Date?
    @State private var jumped = false
    @State private var rendered: Rendered?
    @State private var epoch = 0

    private struct RenderKey: Equatable {
        let spec: ShareCardSpec
        let format: ShareFormat
        let epoch: Int
    }

    private struct Rendered {
        let key: RenderKey
        let image: UIImage
    }

    var body: some View {
        content
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("share-screen")
    }

    @ViewBuilder private var content: some View {
        switch state {
        case .idle, .loading:
            ShareSkeleton()
        case .failed:
            EmptyState(
                title: "Couldn\u{2019}t load this card", actionTitle: "Try again", actionIdentifier: "share-retry",
                systemImage: MonacoIcon.offline.symbol, action: retry
            )
            .frame(maxHeight: .infinity)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("share-failed")
        case .loaded(let spec):
            loaded(spec)
        }
    }

    private func loaded(_ spec: ShareCardSpec) -> some View {
        let key = RenderKey(spec: spec, format: format, epoch: epoch)
        return TimelineView(.animation(paused: replayStart == nil)) { timeline in
            let progress = frozenProgress ?? replayStart.map { ShareReplay.progress(from: $0, at: timeline.date) }
            GeometryReader { proxy in
                let box = CGSize(
                    width: proxy.size.width - MonacoTheme.Space.gutter * 2,
                    height: proxy.size.height - Self.replayHeight - MonacoTheme.Space.s * 2)
                let fitted = Self.fit(format, in: box)
                VStack(spacing: MonacoTheme.Space.s) {
                    stage(spec, fitted: fitted, progress: progress)
                    replayRow(spec, progress: progress)
                }
                .frame(width: proxy.size.width, height: proxy.size.height)
            }
        }
        .safeAreaInset(edge: .bottom, spacing: 0) {
            ShareDock(
                eyes: $eyes, format: $format, unusedEyes: unusedEyes,
                share: { share(spec, current(spec)) }, copyImage: { copyImage(current(spec)) })
        }
        .task(id: spec) {
            await ShareImageExporter.warmCache(for: spec)
            epoch += 1
        }
        .task(id: key) {
            rendered = Rendered(key: key, image: ShareImageExporter.render(spec, format: format))
        }
        .task(id: replayStart) {
            guard replayStart != nil else { return }
            try? await Task.sleep(for: .seconds(ShareReplay.duration))
            if !Task.isCancelled { replayStart = nil }
        }
    }

    private func current(_ spec: ShareCardSpec) -> UIImage {
        if let rendered, rendered.key.spec == spec, rendered.key.format == format { return rendered.image }
        return ShareImageExporter.render(spec, format: format)
    }

    @ViewBuilder
    private func stage(_ spec: ShareCardSpec, fitted: CGSize, progress: Double?) -> some View {
        if let progress {
            ShareExportView(
                spec: ShareReplay.spec(spec, at: progress), format: format, size: fitted, progress: progress)
        } else if let rendered {
            Image(uiImage: rendered.image)
                .resizable()
                .scaledToFit()
                .frame(width: fitted.width, height: fitted.height)
                .accessibilityLabel(spec.spoken)
                .accessibilityAddTraits(.isImage)
                .accessibilityIdentifier("share-card")
        } else {
            SkeletonBlock(width: fitted.width, height: fitted.height, radius: MonacoTheme.Radius.shareCard)
        }
    }

    private func replayRow(_ spec: ShareCardSpec, progress: Double?) -> some View {
        Button {
            Haptics.selection()
            if reduceMotion {
                Task {
                    jumped = true
                    try? await Task.sleep(for: .seconds(1))
                    jumped = false
                }
            } else {
                replayStart = Date()
            }
        } label: {
            HStack(spacing: MonacoTheme.Space.sm) {
                Image(systemName: MonacoIcon.play.symbol)
                    .accessibilityHidden(true)
                Text(spec.replayTitle)
                    .lineLimit(1)
                ShareReplayTrack(progress: jumped ? 1 : (progress ?? 0))
            }
            .font(MonacoTheme.Typo.headline)
            .foregroundStyle(MonacoTheme.primaryText)
            .frame(minHeight: Self.replayHeight)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityIdentifier("share-replay")
    }

    static let replayHeight: CGFloat = MonacoButtonMetrics.hitTarget

    static func fit(_ format: ShareFormat, in box: CGSize) -> CGSize {
        let pixels = ShareImageExporter.pixelSize(format)
        let ratio = pixels.height / pixels.width
        let width = max(min(box.width, box.height / ratio), 0)
        return CGSize(width: width, height: width * ratio)
    }
}

struct ShareReplayTrack: View {
    let progress: Double

    var body: some View {
        Capsule()
            .fill(MonacoTheme.surfaceSunken)
            .frame(height: 4)
            .overlay(alignment: .leading) {
                GeometryReader { proxy in
                    Capsule()
                        .fill(MonacoTheme.primaryText)
                        .frame(width: proxy.size.width * min(max(progress, 0), 1))
                }
            }
            .accessibilityHidden(true)
    }
}
