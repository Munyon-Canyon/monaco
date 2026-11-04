import ImageIO
import MonacoCore
import SwiftUI
import UIKit

private struct GifStillsOnlyKey: EnvironmentKey {
    static let defaultValue = false
}

extension EnvironmentValues {
    var gifStillsOnly: Bool {
        get { self[GifStillsOnlyKey.self] }
        set { self[GifStillsOnlyKey.self] = newValue }
    }
}

struct GifMotion: DynamicProperty {
    @Environment(\.accessibilityReduceMotion) private var systemReduceMotion
    @Environment(\.gifStillsOnly) private var stillsOnly

    var isReduced: Bool { systemReduceMotion || stillsOnly }
}

struct GifPlayButton: View {
    let id: String
    let action: () -> Void

    var body: some View {
        MonacoGlassButton(systemImage: "play.fill", accessibilityLabel: "Play GIF", style: .onCover, action: action)
            .accessibilityIdentifier("gif-play-\(id)")
    }
}

struct GifImageView: View {
    let id: String
    let still: URL
    let animated: URL
    var fills = true
    var showsPlay = true
    var sharedRuns: Binding<Int>?
    var cache: GifDataCache = .shared
    var budget: GifPlaybackBudget?
    var stillShown: Binding<Bool>?

    private let motion = GifMotion()
    @State private var stillImage: UIImage?
    @State private var fileURL: URL?
    @State private var isOnScreen = false
    @State private var hasSlot = false
    @State private var ownRuns = 0
    @State private var didFail = false
    @State private var attempt = 0

    init(
        id: String, still: URL, animated: URL, fills: Bool = true, showsPlay: Bool = true,
        sharedRuns: Binding<Int>? = nil, cache: GifDataCache = .shared, budget: GifPlaybackBudget? = nil,
        stillShown: Binding<Bool>? = nil
    ) {
        self.id = id
        self.still = still
        self.animated = animated
        self.fills = fills
        self.showsPlay = showsPlay
        self.sharedRuns = sharedRuns
        self.cache = cache
        self.budget = budget
        self.stillShown = stillShown
        _stillImage = State(initialValue: Self.initialStill(for: still))
    }

    static func initialStill(for url: URL, store: MonacoRemoteImageStore = GifSheetMetrics.stills) -> UIImage? {
        store.cachedImage(for: url)
    }

    private var runs: Int { sharedRuns?.wrappedValue ?? ownRuns }

    private var reduceMotion: Bool { motion.isReduced }

    private var shouldLoad: Bool {
        isOnScreen && hasSlot && (!reduceMotion || runs > 0)
    }

    private var isPlaying: Bool {
        shouldLoad && fileURL != nil
    }

    var body: some View {
        ZStack {
            stillLayer
            GifFrameView(fileURL: isPlaying ? fileURL : nil, runs: runs, once: reduceMotion, fills: fills)
                .accessibilityHidden(true)
            if didFail && !isPlaying { failureRow }
            if reduceMotion && runs == 0 && showsPlay && sharedRuns == nil && !didFail {
                GifPlayButton(id: id) { ownRuns += 1 }
            }
        }
        .clipped()
        .onAppear {
            isOnScreen = true
            hasSlot = budget?.claim(id) ?? true
        }
        .onDisappear {
            isOnScreen = false
            hasSlot = false
            budget?.release(id)
        }
        .onChange(of: budget?.revision) { _, _ in
            if isOnScreen && !hasSlot { hasSlot = budget?.claim(id) ?? true }
        }
        .task(id: "\(still.absoluteString)#\(attempt)") { await loadStill() }
        .onChange(of: isShowingStill, initial: true) { _, shown in stillShown?.wrappedValue = shown }
        .task(id: "\(shouldLoad)#\(attempt)") {
            guard shouldLoad else { return }
            fileURL = try? await cache.fileURL(for: animated)
        }
    }

    private var isShowingStill: Bool {
        if stillImage != nil { return true }
        #if DEBUG
        return still.host == SampleGifProvider.host
        #else
        return false
        #endif
    }

    private var failureRow: some View {
        Text("GIF didn’t load. Tap to retry.")
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .multilineTextAlignment(.center)
            .minimumScaleFactor(0.5)
            .padding(MonacoTheme.Space.s)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .contentShape(Rectangle())
            .highPriorityGesture(TapGesture().onEnded { retry() })
            .accessibilityIdentifier("gif-still-retry-\(id)")
    }

    private func retry() {
        didFail = false
        attempt += 1
    }

    @ViewBuilder private var stillLayer: some View {
        if let stillImage {
            Image(uiImage: stillImage)
                .resizable()
                .aspectRatio(contentMode: fills ? .fill : .fit)
                .accessibilityHidden(true)
        } else {
            sampleStill
        }
    }

    @ViewBuilder private var sampleStill: some View {
        #if DEBUG
        if still.host == SampleGifProvider.host {
            GifSampleStill(id: id)
        } else {
            Color.clear
        }
        #else
        Color.clear
        #endif
    }

    private func loadStill() async {
        #if DEBUG
        guard still.host != SampleGifProvider.host else { return }
        #endif
        if stillImage == nil { stillImage = GifSheetMetrics.stills.cachedImage(for: still) }
        if stillImage == nil { stillImage = await GifSheetMetrics.stills.image(for: still) }
        if stillImage == nil { didFail = true }
    }
}

private final class GifStopFlag {
    var stopped = false
}

private struct GifFrameView: UIViewRepresentable {
    let fileURL: URL?
    let runs: Int
    let once: Bool
    let fills: Bool

    func makeCoordinator() -> Coordinator {
        Coordinator()
    }

    func makeUIView(context: Context) -> UIView {
        let view = UIView()
        view.layer.masksToBounds = true
        view.layer.contentsGravity = fills ? .resizeAspectFill : .resizeAspect
        view.isAccessibilityElement = false
        return view
    }

    func updateUIView(_ view: UIView, context: Context) {
        context.coordinator.update(view, fileURL: fileURL, runs: runs, once: once)
    }

    static func dismantleUIView(_ view: UIView, coordinator: Coordinator) {
        coordinator.stop()
    }

    final class Coordinator {
        private var flag = GifStopFlag()
        private var key: String?

        func stop() {
            flag.stopped = true
            key = nil
        }

        func update(_ view: UIView, fileURL: URL?, runs: Int, once: Bool) {
            let next = fileURL.map { "\($0.path)#\(runs)" }
            guard next != key else { return }
            flag.stopped = true
            key = next
            guard let fileURL else {
                view.layer.contents = nil
                return
            }
            let flag = GifStopFlag()
            self.flag = flag
            let options: [CFString: Any] = once ? [kCGImageAnimationLoopCount: 1] : [:]
            _ = CGAnimateImageAtURLWithBlock(fileURL as CFURL, options as CFDictionary) { [weak view] _, image, halt in
                guard !flag.stopped, let view else {
                    halt.pointee = true
                    return
                }
                view.layer.contents = image
            }
        }
    }
}
