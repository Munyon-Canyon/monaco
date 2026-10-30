import MonacoAPI
import MonacoCore
import SwiftUI

enum GifSheetMetrics {
    static let gap = MonacoTheme.Space.s
    static let radius = MonacoTheme.Radius.card
    static let placeholderAspects: [CGFloat] = [1, 0.56, 1.5, 0.75, 1.33, 0.5]
    static let stills = MonacoRemoteImageStore(maxPixelSize: 480, countLimit: 120)

    static func columns(_ gifs: [Gif]) -> [[Gif]] {
        var columns: [[Gif]] = [[], []]
        var heights: [CGFloat] = [0, 0]
        for gif in gifs {
            let index = heights[0] <= heights[1] ? 0 : 1
            columns[index].append(gif)
            heights[index] += aspect(gif)
        }
        return columns
    }

    static func aspect(_ gif: Gif) -> CGFloat {
        CGFloat(gif.still.height) / CGFloat(max(gif.still.width, 1))
    }
}

struct GifSheet: View {
    let onPick: (Components.Schemas.CommentAttachmentInput) -> Void

    @State private var model: GifSearchModel
    @Environment(\.dismiss) private var dismiss
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init(
        gifs: any GifProvider, query: String = "",
        onPick: @escaping (Components.Schemas.CommentAttachmentInput) -> Void
    ) {
        self.onPick = onPick
        let model = GifSearchModel(gifs: gifs, clock: ContinuousClock())
        model.query = query
        _model = State(initialValue: model)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            header
            MonacoSearchField(placeholder: "Search GIFs", text: $model.query)
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("gif-search")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            content
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(MonacoTheme.canvas.ignoresSafeArea())
        .presentationDetents([.medium])
        .presentationDragIndicator(.visible)
        .presentationContentInteraction(.scrolls)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("gif-sheet")
        .task { model.load() }
    }

    private var header: some View {
        let stacked = dynamicTypeSize >= .xxxLarge
        let layout =
            stacked
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs))
            : AnyLayout(HStackLayout(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s))
        return layout {
            Text(model.isSearching ? "GIFs" : "Trending GIFs")
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
                .accessibilityAddTraits(.isHeader)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("gif-title")
            if !stacked { Spacer(minLength: 0) }
            Text(model.attribution)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("gif-attribution")
            MonacoGlassButton(systemImage: "xmark", accessibilityLabel: "Close", style: .quiet, size: .compact) {
                dismiss()
            }
            .accessibilityIdentifier("gif-close")
        }
        .frame(minHeight: 48)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
    }

    @ViewBuilder private var content: some View {
        switch model.phase {
        case .idle, .loading:
            GifGrid(placeholders: true, columns: [[], []], onPick: { _ in }, onEnd: {})
        case .failed:
            MonacoNotice(
                .error, message: GifSearchModel.unavailableCopy, action: ("Retry", { model.retry() }),
                actionIdentifier: "gif-retry"
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("gif-error")
        case .loaded(let gifs) where gifs.isEmpty:
            Text("No GIFs for “\(model.normalized)”")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(maxWidth: .infinity)
                .padding(.top, MonacoTheme.Space.l)
                .accessibilityIdentifier("gif-empty")
        case .loaded(let gifs):
            GifGrid(
                placeholders: false, columns: GifSheetMetrics.columns(gifs), isLoadingMore: model.isLoadingMore,
                onPick: pick, onEnd: { model.loadMore() })
        }
    }

    private func pick(_ gif: Gif) {
        onPick(model.select(gif))
        dismiss()
    }
}

private struct GifGrid: View {
    let placeholders: Bool
    let columns: [[Gif]]
    var isLoadingMore = false
    let onPick: (Gif) -> Void
    let onEnd: () -> Void

    var body: some View {
        ScrollView {
            HStack(alignment: .top, spacing: GifSheetMetrics.gap) {
                if placeholders {
                    placeholderColumn(0)
                    placeholderColumn(1)
                } else {
                    ForEach(columns.indices, id: \.self) { column(columns[$0]) }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            if !placeholders {
                Color.clear
                    .frame(height: 1)
                    .onAppear(perform: onEnd)
                if isLoadingMore { ProgressView().padding(.vertical, MonacoTheme.Space.m) }
            }
        }
        .scrollDismissesKeyboard(.interactively)
    }

    private func column(_ gifs: [Gif]) -> some View {
        LazyVStack(spacing: GifSheetMetrics.gap) {
            ForEach(gifs) { gif in
                Button {
                    onPick(gif)
                } label: {
                    GifStill(gif: gif)
                        .accessibilityElement(children: .ignore)
                }
                .buttonStyle(.plain)
                .accessibilityLabel(gif.title.isEmpty ? "GIF" : gif.title)
                .accessibilityIdentifier("gif-cell-\(gif.id)")
            }
        }
    }

    private func placeholderColumn(_ index: Int) -> some View {
        let aspects = GifSheetMetrics.placeholderAspects
        return VStack(spacing: GifSheetMetrics.gap) {
            ForEach(0..<3, id: \.self) { row in
                RoundedRectangle(cornerRadius: GifSheetMetrics.radius, style: .continuous)
                    .fill(MonacoTheme.surfaceSunken)
                    .aspectRatio(1 / aspects[(row * 2 + index) % aspects.count], contentMode: .fit)
            }
        }
        .accessibilityHidden(true)
    }
}

private struct GifStill: View {
    let gif: Gif

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: GifSheetMetrics.radius, style: .continuous)
        shape
            .fill(MonacoTheme.surfaceSunken)
            .aspectRatio(1 / GifSheetMetrics.aspect(gif), contentMode: .fit)
            .overlay {
                GifImageView(
                    id: gif.id, still: gif.still.url, animated: gif.preview.url, showsPlay: false,
                    budget: .sheet)
            }
            .clipShape(shape)
            .contentShape(shape)
    }
}

struct GifPicker: ViewModifier {
    @Binding var isPresented: Bool
    let model: CommentsModel

    @Environment(AppEnvironment.self) private var environment: AppEnvironment?

    func body(content: Content) -> some View {
        content.sheet(isPresented: $isPresented) {
            GifSheet(gifs: environment?.gifs ?? UnavailableGifProvider()) { attachment in
                Task { _ = await model.post(CommentDraft(text: model.draft), attachment: attachment) }
            }
        }
    }
}

extension View {
    func gifPicker(isPresented: Binding<Bool>, model: CommentsModel) -> some View {
        modifier(GifPicker(isPresented: isPresented, model: model))
    }
}
