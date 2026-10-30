import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct MembersRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        MembersView(cabalID: cabalID)
    }
}

struct MembersView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var models = CabalScreenModels()
    @State private var retryTick = 0

    var body: some View {
        let (cabal, _) = models.prepared(cabalID: cabalID, environment: environment)
        content(cabal)
            .navigationTitle("Members")
            .navigationBarTitleDisplayMode(.inline)
            .monacoCanvas()
            .task(id: retryTick) {
                await cabal.load()
                await cabal.observe()
            }
            .onScreenVisibilityChange { cabal.setVisible($0) }
    }

    @ViewBuilder private func content(_ cabal: CabalModel) -> some View {
        switch cabal.state {
        case .idle, .loading:
            MembersSkeleton()
        case .failed:
            CabalFullScreenError(identifier: "members-failed") { retryTick += 1 }
        case .loaded(let loaded):
            CabalMembersHost(cabal: loaded) { model in
                MembersAllList(cabal: loaded, model: model)
            }
        }
    }
}

private struct MembersAllList: View {
    let cabal: Components.Schemas.Cabal
    let model: CabalMembersModel

    private enum Item: Identifiable {
        case request(CabalPendingRequest)
        case header(String)
        case member(CabalMemberEntry, group: CabalMemberGroup)
        case invite
        case footer

        var id: String {
            switch self {
            case .request(let request): "request-\(request.id)"
            case .header(let title): "header-\(title)"
            case .member(let entry, _): "member-\(entry.id)"
            case .invite: "invite"
            case .footer: "footer"
            }
        }
    }

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    @State private var showsInvite = false

    private var items: [Item] {
        var items = model.requests.map(Item.request)
        var last: CabalMemberGroup?
        for entry in model.rows {
            let group = CabalMemberGroup(entry.row)
            if group != last, let totals = model.totals {
                items.append(.header(CabalMemberCopy.sectionTitle(group, totals: totals)))
                last = group
            }
            items.append(.member(entry, group: group))
        }
        return items + [.invite, .footer]
    }

    var body: some View {
        CabalMembersList(model: model) {
            List {
                ForEach(items) { item in
                    row(item)
                        .listRowInsets(EdgeInsets())
                        .listRowSeparator(.hidden)
                        .listRowBackground(Color.clear)
                }
                CabalMembersMore(model: model)
                    .listRowInsets(EdgeInsets())
                    .listRowSeparator(.hidden)
                    .listRowBackground(Color.clear)
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            .refreshable { await model.loadFirst() }
        }
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    showsInvite = true
                } label: {
                    Image(systemName: "person.badge.plus")
                        .frame(minWidth: MonacoButtonMetrics.hitTarget, minHeight: MonacoButtonMetrics.hitTarget)
                        .contentShape(Rectangle())
                }
                .accessibilityLabel(CabalMemberCopy.invite)
                .accessibilityIdentifier("members-invite-button")
            }
        }
        .sheet(isPresented: $showsInvite) { CabalInviteSheet(cabal: cabal) }
        .accessibilityIdentifier("members-list")
    }

    private func open(_ row: Components.Schemas.CabalMemberRow) {
        let preview = UserPreview(displayName: row.displayName, handle: row.handle, photoURL: row.photoUrl)
        environment.navigator.open(
            UserProfileRoute(userID: row.userId, preview: preview),
            in: hostMainTab ?? environment.navigator.selectedTab)
    }

    @ViewBuilder private func row(_ item: Item) -> some View {
        let viewerID = environment.viewer?.userID
        switch item {
        case .request(let request):
            MembersRequestRow(
                request: request, isDeciding: model.deciding.contains(request.id),
                approve: { Task { await model.approve(request.id) } },
                decline: { Task { await model.deny(request.id) } })
        case .header(let title):
            MonacoSectionHeader(title)
                .padding(.horizontal, MonacoTheme.Space.gutter)
        case .member(let entry, _):
            Button {
                open(entry.row)
            } label: {
                CabalMemberLine(
                    entry: entry, viewerID: viewerID, subtitle: CabalMemberCopy.voterLine(entry.row),
                    chevron: true, isLast: entry.id == model.rows.last?.id)
            }
            .buttonStyle(.monacoRow)
            .onAppear { Task { await model.loadMore(after: entry) } }
        case .invite:
            Button {
                showsInvite = true
            } label: {
                MonacoRow(
                    title: CabalMemberCopy.inviteTo(CabalHeaderCopy.shortName(cabal.name)),
                    titleColor: MonacoTheme.brand, isLast: true,
                    leading: { SunkenGlyphMark(systemImage: "person.badge.plus") },
                    trailing: { EmptyView() })
            }
            .buttonStyle(.monacoRow)
            .tintedCard()
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("members-invite-row")
        case .footer:
            Text(CabalMemberCopy.membersFooter)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.vertical, MonacoTheme.Space.m)
                .accessibilityIdentifier("members-footer")
        }
    }
}

private struct MembersRequestRow: View {
    let request: CabalPendingRequest
    let isDeciding: Bool
    let approve: () -> Void
    let decline: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var layout: MonacoRowLayout { MonacoRowLayout(dynamicTypeSize: dynamicTypeSize) }

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.s)
            .frame(minHeight: MonacoRowLayout.minHeight)
            .contentShape(Rectangle())
            .overlay(alignment: .bottom) {
                MonacoRule()
                    .padding(.leading, layout.separatorLeadingInset)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            }
            .contextMenu {
                Button(CabalMemberCopy.decline, role: .destructive, action: decline)
                    .accessibilityIdentifier("cabal-join-deny")
            }
            .swipeActions(edge: .trailing) {
                Button(CabalMemberCopy.decline, role: .destructive, action: decline)
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabal-join-request-row")
    }

    @ViewBuilder private var content: some View {
        if layout.isStacked {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    avatar
                    name.frame(maxWidth: .infinity, alignment: .leading)
                }
                line
                approveButton
            }
        } else {
            HStack(spacing: MonacoTheme.Space.sm) {
                avatar
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    name
                    line
                }
                .frame(minWidth: layout.minimumTitleWidth, maxWidth: .infinity, alignment: .leading)
                approveButton.layoutPriority(1)
            }
        }
    }

    private var avatar: some View {
        MonacoAvatar(
            photoURL: request.photoURL, displayName: request.name, size: MonacoRowLayout.baseMarkSize,
            seed: request.userID
        )
        .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
    }

    private var name: some View {
        Text(request.name)
            .font(MonacoTheme.Typo.rowTitle)
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(2)
            .monacoPrivate(true)
    }

    private var line: some View {
        Text(CabalMemberCopy.requestLine(via: request.via, createdAt: request.createdAt, now: .now))
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .fixedSize(horizontal: false, vertical: true)
            .monacoPrivate(true)
    }

    private var approveButton: some View {
        Button(CabalMemberCopy.approve, action: approve)
            .buttonStyle(.monacoCompact)
            .disabled(isDeciding)
            .accessibilityLabel("\(CabalMemberCopy.approve) \(request.name)")
            .accessibilityIdentifier("cabal-join-approve")
    }
}

private struct MembersSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MonacoRowSkeleton(rows: 4, markShape: .circle, hasTrailing: true)
            Spacer(minLength: 0)
        }
        .padding(.top, MonacoTheme.Space.sm)
        .accessibilityElement()
        .accessibilityLabel("Loading members")
        .accessibilityIdentifier("members-loading")
    }
}
