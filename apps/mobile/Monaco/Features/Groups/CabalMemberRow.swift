import MonacoAPI
import MonacoCore
import SwiftUI

struct CabalMemberLine: View {
    let entry: CabalMemberEntry
    let viewerID: String?
    let subtitle: String
    var chevron = false
    var isLast = false

    var body: some View {
        let row = entry.row
        let name = CabalMemberCopy.name(row, viewerID: viewerID)
        MonacoRow(
            title: name, subtitle: subtitle, chevron: chevron, isLast: isLast, isPrivate: true,
            leading: { CabalMemberPhoto(row: row, name: name) },
            trailing: {
                Text(CabalMemberCopy.share(basisPoints: row.ownershipBps))
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
            }
        )
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("cabal-member-\(row.userId)")
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

    var body: some View {
        MonacoAvatar(photoURL: row.photoUrl, displayName: name, size: MonacoRowLayout.baseMarkSize, seed: row.userId)
            .overlay(alignment: .top) {
                if row.crown {
                    Image(systemName: "crown.fill")
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.goldGlyph)
                        .offset(y: -MonacoTheme.Space.s)
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
