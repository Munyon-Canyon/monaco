import MonacoAPI
import MonacoCore
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

private struct CabalHeader: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalRetry) private var retry
    @Environment(\.cabalModel) private var model

    var body: some View {
        CabalHero(
            model: model,
            pictureWriter: LiveCabalPictureWriter(api: environment.api),
            retry: {
                if let again = retry.retry { again() } else { Task { await model?.load() } }
            },
            pictureResult: { toast in
                toasts.current = toast
                guard toast.isSuccess else { return }
                Task { await model?.load() }
            }
        )
        .onChange(of: model?.failureTick) { _, _ in
            guard model?.cabal != nil, let error = model?.lastError else { return }
            toasts.show(error)
        }
    }
}

private struct CabalHero: View {
    let model: CabalModel?
    let pictureWriter: any CabalPictureWriting
    let retry: () -> Void
    let pictureResult: (MonacoToast) -> Void
    @Environment(\.sectionScrollProxy) private var scrollProxy

    static let visibleFaces = 5

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.sm)
            .padding(.bottom, CabalInkBand<EmptyView>.rhythm)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(MonacoTheme.heroInk)
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            HStack(spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(width: 48, height: 48, radius: MonacoTheme.Radius.card)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text("Cabal").font(MonacoTheme.Typo.title).skeletonBar(width: 160)
                    SkeletonBlock(width: 96, height: 14).frame(height: 44, alignment: .leading)
                }
            }
            .accessibilityElement()
            .accessibilityLabel("Loading this cabal")
            .accessibilityIdentifier("cabal-header-loading")
        case .failed:
            MonacoErrorRow(thing: "this cabal", identifier: "cabal-header-failed", inset: false, retry: retry)
        case .loaded(let cabal):
            identity(cabal)
        }
    }

    private func identity(_ cabal: Components.Schemas.Cabal) -> some View {
        HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
            if CabalHeaderSlot.canChangePicture(cabal) {
                CreatorPictureTile(cabal: cabal, writer: pictureWriter, onResult: pictureResult)
            } else {
                CabalMark(
                    groupId: cabal.id,
                    name: cabal.name,
                    size: 48,
                    pictureUrl: cabal.pictureUrl,
                    accessibilityLabel: cabal.pictureUrl == nil ? nil : "\(cabal.name) picture"
                )
            }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(cabal.name)
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.onHero)
                    .lineLimit(3)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityIdentifier("cabal-header-name")
                memberRow(cabal)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-header")
    }

    private func memberRow(_ cabal: Components.Schemas.Cabal) -> some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: MonacoTheme.Space.s) {
                memberFaces(cabal)
                memberCount(cabal)
            }
            VStack(alignment: .leading, spacing: 0) {
                memberFaces(cabal)
                memberCount(cabal)
            }
        }
    }

    private func memberFaces(_ cabal: Components.Schemas.Cabal) -> some View {
        Button(action: showMemberBoard) {
            MemberFaces(members: Array(cabal.members.prefix(Self.visibleFaces)))
                .frame(minHeight: 44)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("See the leaderboard")
        .accessibilityIdentifier("cabal-header-faces")
    }

    private func memberCount(_ cabal: Components.Schemas.Cabal) -> some View {
        Text(CabalCopy.memberCount(cabal.memberCount))
            .font(MonacoTheme.Typo.callout)
            .foregroundStyle(MonacoTheme.onHeroMuted)
            .lineLimit(1)
            .fixedSize(horizontal: true, vertical: true)
            .frame(minHeight: 44)
            .contentShape(Rectangle())
            .onTapGesture(perform: showMemberBoard)
            .accessibilityIdentifier("cabal-member-count")
    }

    private func showMemberBoard() {
        withAnimation { scrollProxy?.scrollTo("cabal-member-board", anchor: .top) }
    }
}

private struct CreatorPictureTile: View {
    let cabal: Components.Schemas.Cabal
    let onResult: (MonacoToast) -> Void
    @StateObject private var editor: CabalPictureEditor

    init(cabal: Components.Schemas.Cabal, writer: any CabalPictureWriting, onResult: @escaping (MonacoToast) -> Void) {
        self.cabal = cabal
        self.onResult = onResult
        _editor = StateObject(
            wrappedValue: CabalPictureEditor(groupId: cabal.id, pictureUrl: cabal.pictureUrl, writer: writer)
        )
    }

    var body: some View {
        CabalPicturePicker(
            groupId: cabal.id,
            name: cabal.name,
            canEdit: true,
            size: 48,
            showsRemoveButton: false,
            onResult: onResult,
            editor: editor
        )
        .frame(minWidth: 48, minHeight: 48)
        .accessibilityLabel("Change picture")
        .accessibilityIdentifier("cabal-header-picture")
        .onChange(of: cabal.pictureUrl) { _, refreshed in
            editor.adoptFromRefresh(refreshed)
        }
    }
}

private struct MemberFaces: View {
    let members: [Components.Schemas.CabalMember]

    private static let size: CGFloat = 22

    var body: some View {
        HStack(spacing: -4) {
            ForEach(members, id: \.userId) { member in
                MonacoAvatar(
                    photoURL: member.photoUrl,
                    displayName: CabalCopy.memberName(member),
                    size: Self.size,
                    seed: member.userId
                )
                .overlay(Circle().strokeBorder(MonacoTheme.heroInk, lineWidth: 2))
            }
        }
        .accessibilityHidden(true)
    }
}
