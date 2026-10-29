import MonacoAPI
import MonacoCore
import SwiftUI

struct CabalMemberLine: View {
    let entry: CabalMemberEntry
    let viewerID: String?
    let subtitle: String
    var chevron = false
    var isLast = false

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let row = entry.row
        let name = CabalMemberCopy.name(row, viewerID: viewerID)
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                stacked(row: row, name: name)
            } else {
                MonacoRow(
                    title: name, subtitle: subtitle, subtitleLineLimit: dynamicTypeSize >= .xxLarge ? 6 : 1,
                    chevron: chevron, isLast: isLast, isPrivate: true,
                    leading: { CabalMemberPhoto(row: row, name: name) },
                    trailing: { share(row) }
                )
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("cabal-member-\(row.userId)")
    }

    private func share(_ row: Components.Schemas.CabalMemberRow) -> some View {
        Text(CabalMemberCopy.share(basisPoints: row.ownershipBps))
            .font(MonacoTheme.Typo.rowTitle)
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(1)
    }

    private func stacked(row: Components.Schemas.CabalMemberRow, name: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: MonacoTheme.Space.sm) {
                CabalMemberPhoto(row: row, name: name)
                    .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
                Text(name)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(2)
                    .monacoPrivate(true)
                    .frame(maxWidth: .infinity, alignment: .leading)
                if chevron { MonacoRowChevron() }
            }
            share(row)
            if !subtitle.isEmpty {
                Text(subtitle)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
                    .monacoPrivate(true)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity, minHeight: MonacoRowLayout.minHeight, alignment: .leading)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
    }
}

struct CabalMemberLink: View {
    let entry: CabalMemberEntry
    let viewerID: String?
    let subtitle: String
    var chevron = false
    var isLast = false

    var body: some View {
        let row = entry.row
        NavigationLink(
            value: AnyAppRoute(
                UserProfileRoute(
                    userID: row.userId,
                    preview: UserPreview(displayName: row.displayName, handle: row.handle, photoURL: row.photoUrl)))
        ) {
            CabalMemberLine(entry: entry, viewerID: viewerID, subtitle: subtitle, chevron: chevron, isLast: isLast)
        }
        .buttonStyle(.monacoRow)
    }
}

struct CabalMemberPhoto: View {
    let row: Components.Schemas.CabalMemberRow
    let name: String

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        MonacoAvatar(photoURL: row.photoUrl, displayName: name, size: MonacoRowLayout.baseMarkSize, seed: row.userId)
            .overlay(alignment: dynamicTypeSize.isAccessibilitySize ? .topTrailing : .top) {
                if row.crown {
                    Image(systemName: "crown.fill")
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.goldGlyph)
                        .offset(y: dynamicTypeSize.isAccessibilitySize ? 0 : -MonacoTheme.Space.s)
                        .accessibilityLabel("Month leader")
                        .accessibilityIdentifier("cabal-member-crown-\(row.userId)")
                }
            }
    }
}

struct CabalMembersHost<Content: View>: View {
    let cabal: Components.Schemas.Cabal
    @ViewBuilder let content: (CabalMembersModel) -> Content

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: CabalMembersModel?

    private struct Key: Hashable {
        let id: String
        let role: String?
    }

    var body: some View {
        Group {
            if let model {
                content(model)
            } else {
                MonacoRowSkeleton(rows: 4, markShape: .circle, hasTrailing: true)
            }
        }
        .task(id: Key(id: cabal.id, role: cabal.me?.role)) {
            let model = CabalMembersModel(cabal: cabal, api: environment.api, hints: environment.hints)
            self.model = model
            await model.loadFirst()
            await model.observe()
        }
        .onChange(of: model?.toast) { _, toast in
            guard let toast else { return }
            toasts.current = MonacoToast(message: toast.message, isSuccess: toast.isSuccess)
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }
}

struct CabalMembersList<Rows: View>: View {
    let model: CabalMembersModel
    @ViewBuilder let rows: () -> Rows

    var body: some View {
        if model.rows.isEmpty, case .failed = model.phase {
            MonacoErrorRow(thing: "members", identifier: "members-list-failed") {
                Task { await model.loadFirst() }
            }
        } else if model.rows.isEmpty {
            MonacoRowSkeleton(rows: 4, markShape: .circle, hasTrailing: true)
        } else {
            rows()
        }
    }
}

struct CabalMembersMore: View {
    let model: CabalMembersModel

    var body: some View {
        switch model.phase {
        case .loadingMore:
            MonacoRowSkeleton(rows: 1, markShape: .circle, hasTrailing: true)
        case .failed:
            MonacoErrorRow(thing: "more members", identifier: "members-more-failed") {
                Task { await model.retryMore() }
            }
        default:
            EmptyView()
        }
    }
}
