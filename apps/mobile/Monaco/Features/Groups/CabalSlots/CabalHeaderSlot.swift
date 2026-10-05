import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalHeaderSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalHeader(cabalID: context.cabalID)
    }

    static func canChangePicture(_ cabal: Components.Schemas.Cabal) -> Bool {
        cabal.me?.role == "creator"
    }
}

private struct CabalHeader: View {
    let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalRetry) private var retry
    @State private var model: CabalModel?

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
        .task(id: retry.tick) {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: model?.failureTick) { _, _ in
            guard model?.cabal != nil, let error = model?.lastError else { return }
            toasts.show(error)
        }
    }

    private func preparedModel() -> CabalModel {
        if let model { return model }
        let created = CabalModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        model = created
        return created
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
            .padding(.bottom, MonacoTheme.Space.l)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(MonacoTheme.heroInk)
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            HStack(spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(width: 48, height: 48, radius: MonacoTheme.Radius.card)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    SkeletonBlock(width: 160, height: 22)
                    SkeletonBlock(width: 96, height: 14)
                }
            }
            .accessibilityElement()
            .accessibilityLabel("Loading this cabal")
            .accessibilityIdentifier("cabal-header-loading")
        case .failed:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                Text("Couldn't load this cabal.")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.onHero)
                Button("Try again", action: retry)
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("cabal-header-retry")
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabal-header-failed")
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
                    onInk: true,
                    pictureUrl: cabal.pictureUrl,
                    accessibilityLabel: cabal.pictureUrl == nil ? nil : "\(cabal.name) picture"
                )
            }
            VStack(alignment: .leading, spacing: 4) {
                Text(cabal.name)
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.onHero)
                    .lineLimit(2)
                    .minimumScaleFactor(0.75)
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityIdentifier("cabal-header-name")
                HStack(spacing: MonacoTheme.Space.s) {
                    Button(action: showMemberBoard) {
                        MemberFaces(members: Array(cabal.members.prefix(Self.visibleFaces)))
                            .frame(minHeight: 44)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("See the leaderboard")
                    .accessibilityIdentifier("cabal-header-faces")
                    Text(CabalCopy.memberCount(cabal.memberCount))
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.onHeroMuted)
                        .frame(minHeight: 44)
                        .contentShape(Rectangle())
                        .onTapGesture(perform: showMemberBoard)
                        .accessibilityIdentifier("cabal-member-count")
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-header")
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
            onInk: true,
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
