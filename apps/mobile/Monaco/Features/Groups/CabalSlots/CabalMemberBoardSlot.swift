import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalMemberBoardSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalMemberBoard(cabalID: context.cabalID)
            .id("cabal-member-board")
    }

    static func ordered(_ members: [Components.Schemas.CabalMember]) -> [Components.Schemas.CabalMember] {
        members.enumerated()
            .sorted { lhs, rhs in
                if lhs.element.joinedAt != rhs.element.joinedAt { return lhs.element.joinedAt < rhs.element.joinedAt }
                let lhsCreator = lhs.element.role == "creator"
                let rhsCreator = rhs.element.role == "creator"
                if lhsCreator != rhsCreator { return lhsCreator }
                return lhs.offset < rhs.offset
            }
            .map(\.element)
    }
}

private struct CabalMemberBoard: View {
    let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalRetry) private var retry
    @State private var model: CabalModel?

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Leaderboard")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            content
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .contain)
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

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            BoardRowSkeleton(rows: 3)
        case .failed:
            HStack {
                Text("Couldn't load the members.")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
                Spacer()
                Button("Try again") {
                    Task { await model?.load() }
                }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("cabal-member-board-retry")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        case .loaded(let cabal):
            rows(CabalMemberBoardSlot.ordered(cabal.members))
            Text("Rankings show up here soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cabal-member-board-coming")
        }
    }

    private func rows(_ members: [Components.Schemas.CabalMember]) -> some View {
        VStack(spacing: 0) {
            ForEach(Array(members.enumerated()), id: \.element.userId) { index, member in
                NavigationLink(
                    value: AnyAppRoute(
                        UserProfileRoute(
                            userID: member.userId,
                            preview: UserPreview(
                                displayName: member.displayName, handle: member.handle, photoURL: member.photoUrl)
                        ))
                ) {
                    CabalMemberRow(
                        member: member,
                        isViewer: member.userId == environment.viewer?.userID,
                        isLast: index == members.count - 1
                    )
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("cabal-member-\(member.userId)")
            }
        }
        .overlay(alignment: .top) { MonacoRule() }
        .overlay(alignment: .bottom) { MonacoRule() }
    }

    private func preparedModel() -> CabalModel {
        if let model { return model }
        let created = CabalModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}

private struct CabalMemberRow: View {
    let member: Components.Schemas.CabalMember
    let isViewer: Bool
    let isLast: Bool

    private static let faceSize: CGFloat = 40

    var body: some View {
        let name = CabalCopy.memberName(member)
        HStack(spacing: MonacoTheme.Space.sm) {
            MonacoAvatar(photoURL: member.photoUrl, displayName: name, size: Self.faceSize, seed: member.userId)
            Text(name)
                .font(MonacoTheme.Typo.rowTitle)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
                .frame(maxWidth: .infinity, alignment: .leading)
            if isViewer {
                Text("You")
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.brandOnWash)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: 60)
        .background(isViewer ? MonacoTheme.brandWash : Color.clear)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.leading, MonacoTheme.Space.gutter + Self.faceSize + MonacoTheme.Space.sm)
            }
        }
        .accessibilityElement(children: .combine)
    }
}
