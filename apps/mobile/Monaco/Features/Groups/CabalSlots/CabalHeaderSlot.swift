import MonacoAPI
import MonacoCore
import PhotosUI
import SwiftUI

enum CabalHeaderSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalHeader()
    }

    static func canChangePicture(_ cabal: Components.Schemas.Cabal) -> Bool {
        cabal.me?.role == "creator"
    }
}

enum CabalHeroMetrics {
    static let nameInset: CGFloat = 20
    static let bottomInset: CGFloat = 54
    static let skeletonHeight: CGFloat = 132
    static let tallSkeletonHeight: CGFloat = 172
    static let discLeading: CGFloat = 10
    static let discBottom: CGFloat = 8
}

private struct CabalHeader: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalRetry) private var retry
    @Environment(\.cabalModel) private var model
    @Environment(\.cabalShare) private var share

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.s)
            .onChange(of: model?.failureTick) { _, _ in
                guard model?.cabal != nil, let error = model?.lastError else { return }
                toasts.show(error)
            }
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            SkeletonBlock(height: CabalHeroMetrics.skeletonHeight, radius: MonacoTheme.Radius.identity)
                .accessibilityElement()
                .accessibilityLabel("Loading this cabal")
                .accessibilityIdentifier("cabal-header-loading")
        case .failed:
            MonacoErrorRow(thing: "this cabal", identifier: "cabal-header-failed", inset: false) {
                if let again = retry.retry { again() } else { Task { await model?.load() } }
            }
        case .loaded(let cabal):
            CabalHeroCover(
                cabal: cabal,
                pictureWriter: LiveCabalPictureWriter(api: environment.api),
                share: share?.open,
                pictureResult: { toast in
                    toasts.current = toast
                    guard toast.isSuccess else { return }
                    Task { await model?.load() }
                })
        }
    }
}

struct CabalHeroCover: View {
    let cabal: Components.Schemas.Cabal
    var subtitle: String?
    var tall = false
    var pictureWriter: (any CabalPictureWriting)?
    var share: (() -> Void)?
    var pictureResult: (MonacoToast) -> Void = { _ in }

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @StateObject private var editor: CabalPictureEditor
    @State private var selection: PhotosPickerItem?
    @State private var confirmingRemoval = false

    init(
        cabal: Components.Schemas.Cabal,
        subtitle: String? = nil,
        tall: Bool = false,
        pictureWriter: (any CabalPictureWriting)? = nil,
        share: (() -> Void)? = nil,
        pictureResult: @escaping (MonacoToast) -> Void = { _ in }
    ) {
        self.cabal = cabal
        self.subtitle = subtitle
        self.tall = tall
        self.pictureWriter = pictureWriter
        self.share = share
        self.pictureResult = pictureResult
        _editor = StateObject(
            wrappedValue: CabalPictureEditor(
                groupId: cabal.id, pictureUrl: cabal.pictureUrl, writer: pictureWriter ?? NoPictureWriter()))
    }

    private var canChangePicture: Bool { pictureWriter != nil && CabalHeaderSlot.canChangePicture(cabal) }

    var body: some View {
        cover
            .overlay { if canChangePicture { picker } }
            .overlay(alignment: .bottomLeading) { if let share { disc(share) } }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabal-header")
            .onChange(of: cabal.pictureUrl) { _, refreshed in editor.adoptFromRefresh(refreshed) }
            .onChange(of: selection) { _, item in
                guard let item else { return }
                Task {
                    pictureResult(await editor.upload(item))
                    selection = nil
                }
            }
            .confirmationDialog(
                "Remove the cabal picture?", isPresented: $confirmingRemoval, titleVisibility: .visible
            ) {
                Button("Remove picture", role: .destructive) {
                    Task { pictureResult(await editor.remove()) }
                }
                .accessibilityIdentifier("cabal-picture-remove-confirm")
            }
    }

    private var cover: some View {
        CabalCover(
            cabalId: cabal.id, tint: .forGroupId(cabal.id), pictureURL: CabalMark.resolvedURL(editor.pictureUrl),
            style: tall ? .heroTall : .hero
        ) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(cabal.name)
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.onCover)
                    .lineLimit(dynamicTypeSize.isAccessibilitySize ? 3 : 1)
                    .truncationMode(.tail)
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityIdentifier("cabal-header-name")
                Text(subtitle ?? CabalCopy.memberCount(cabal.memberCount))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.onCoverMuted)
                    .accessibilityIdentifier("cabal-member-count")
            }
            .padding(.horizontal, CabalHeroMetrics.nameInset)
            .padding(.top, CabalHeroMetrics.nameInset)
            .padding(.bottom, share == nil ? CabalHeroMetrics.nameInset : CabalHeroMetrics.bottomInset)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var picker: some View {
        PhotosPicker(selection: $selection, matching: .images, photoLibrary: .shared()) {
            Color.clear.contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(editor.isWorking)
        .accessibilityLabel(editor.pictureUrl == nil ? "Add cabal picture" : "Change cabal picture")
        .accessibilityHint("Opens your photo library")
        .accessibilityIdentifier("cabal-header-picture")
        .contextMenu {
            if editor.pictureUrl != nil {
                Button(role: .destructive) {
                    confirmingRemoval = true
                } label: {
                    Label("Remove picture", systemImage: MonacoIcon.trash.symbol)
                }
                .accessibilityIdentifier("cabal-picture-remove")
            }
        }
    }

    private func disc(_ action: @escaping () -> Void) -> some View {
        CoverShareDisc(cabalName: cabal.name, action: action)
            .padding(.leading, CabalHeroMetrics.discLeading)
            .padding(.bottom, CabalHeroMetrics.discBottom)
            .accessibilityLabel("Share cabal invitation")
            .accessibilityIdentifier("cabal-hero-share")
    }
}

@MainActor
private struct NoPictureWriter: CabalPictureWriting {
    func uploadPicture(groupId: String, imageData: Data, mimeType: String) async throws -> String? { nil }
    func removePicture(groupId: String) async throws -> String? { nil }
}
