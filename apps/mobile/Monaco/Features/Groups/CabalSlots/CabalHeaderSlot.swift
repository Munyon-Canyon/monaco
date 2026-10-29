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

struct CabalHeaderNameBottom: PreferenceKey {
    static let defaultValue: CGFloat? = nil

    static func reduce(value: inout CGFloat?, nextValue: () -> CGFloat?) {
        value = nextValue() ?? value
    }
}

enum CabalHeaderTitle {
    static func isShown(nameBottom: CGFloat?, barBottom: CGFloat) -> Bool {
        guard let nameBottom else { return false }
        return nameBottom <= barBottom
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
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(height: CabalHeroMetrics.tallSkeletonHeight, radius: MonacoTheme.Radius.identity)
                HStack(spacing: MonacoTheme.Space.sm) {
                    SkeletonBlock(
                        width: CabalMark.Size.row.rawValue, height: CabalMark.Size.row.rawValue,
                        radius: MonacoTheme.Radius.tile)
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                        Text(" ").font(MonacoTheme.Typo.title).skeletonBar(width: 220)
                        Text(" ").font(MonacoTheme.Typo.caption).skeletonBar(width: 160)
                    }
                }
            }
            .accessibilityElement()
            .accessibilityLabel("Loading this cabal")
            .accessibilityIdentifier("cabal-header-loading")
        case .failed:
            MonacoErrorRow(thing: "this cabal", identifier: "cabal-header-failed", inset: false) {
                if let again = retry.retry { again() } else { Task { await model?.load() } }
            }
        case .loaded(let cabal):
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                CabalHeroCover(
                    cabal: cabal, tall: true, plain: true,
                    pictureWriter: LiveCabalPictureWriter(api: environment.api),
                    pictureResult: { toast in
                        toasts.current = toast
                        guard toast.isSuccess else { return }
                        Task { await model?.load() }
                    })
                CabalIdentityRow(cabal: cabal)
            }
        }
    }
}

private struct CabalIdentityRow: View {
    let cabal: Components.Schemas.Cabal

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var subline: String {
        CabalHeaderCopy.subline(
            memberCount: cabal.memberCount, createdAt: cabal.createdAt, role: cabal.me?.role,
            canVote: cabal.me?.canVote ?? false, now: .now)
    }

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            CabalMark(groupId: cabal.id, name: cabal.name, size: .row, pictureUrl: cabal.pictureUrl)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(cabal.name)
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(dynamicTypeSize.isAccessibilitySize ? 3 : 2)
                    .truncationMode(.tail)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityIdentifier("cabal-header-name")
                    .background {
                        GeometryReader { proxy in
                            Color.clear.preference(
                                key: CabalHeaderNameBottom.self, value: proxy.frame(in: .global).maxY)
                        }
                    }
                Text(subline)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("cabal-member-count")
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

struct CabalHeroCover: View {
    let cabal: Components.Schemas.Cabal
    var subtitle: String?
    var tall = false
    var plain = false
    var pictureWriter: (any CabalPictureWriting)?
    var share: (() -> Void)?
    var shareBusy = false
    var pictureResult: (MonacoToast) -> Void = { _ in }

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @StateObject private var editor: CabalPictureEditor
    @State private var selection: PhotosPickerItem?
    @State private var confirmingRemoval = false

    init(
        cabal: Components.Schemas.Cabal,
        subtitle: String? = nil,
        tall: Bool = false,
        plain: Bool = false,
        pictureWriter: (any CabalPictureWriting)? = nil,
        share: (() -> Void)? = nil,
        shareBusy: Bool = false,
        pictureResult: @escaping (MonacoToast) -> Void = { _ in }
    ) {
        self.cabal = cabal
        self.subtitle = subtitle
        self.tall = tall
        self.plain = plain
        self.pictureWriter = pictureWriter
        self.share = share
        self.shareBusy = shareBusy
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
            if plain {
                plainOverlay
            } else {
                nameOverlay
            }
        }
    }

    @ViewBuilder private var plainOverlay: some View {
        if CabalMark.resolvedURL(editor.pictureUrl) == nil {
            Text(CabalMark.initials(for: cabal.name))
                .font(MonacoTheme.Typo.display)
                .foregroundStyle(MonacoTheme.onCover)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private var nameOverlay: some View {
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
        CoverShareDisc(cabalName: cabal.name, isBusy: shareBusy, action: action)
            .padding(.leading, CabalHeroMetrics.discLeading)
            .padding(.bottom, CabalHeroMetrics.discBottom)
            .accessibilityLabel("Share cabal")
            .accessibilityIdentifier("cabal-hero-share")
    }
}

@MainActor
private struct NoPictureWriter: CabalPictureWriting {
    func uploadPicture(groupId: String, imageData: Data, mimeType: String) async throws -> String? { nil }
    func removePicture(groupId: String) async throws -> String? { nil }
}
