import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalMembersSlot: CabalSection {
    static let isLive = true

    static func body(for _: CabalContext) -> some View {
        CabalMembersTab()
    }
}

struct CabalMembersTab: View {
    @Environment(\.cabalModel) private var cabalModel

    var body: some View {
        Group {
            if let cabal = cabalModel?.cabal {
                CabalMembersHost(cabal: cabal) { model in
                    CabalMembersTabBody(cabal: cabal, model: model)
                }
            } else if case .failed = cabalModel?.state {
                MonacoErrorRow(thing: "the members", identifier: "cabal-members-failed", inset: false) {
                    Task { await cabalModel?.load() }
                }
            } else {
                MonacoRowSkeleton(rows: 4, markShape: .circle, hasTrailing: true)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-members")
    }
}

private struct CabalMembersTabBody: View {
    let cabal: Components.Schemas.Cabal
    let model: CabalMembersModel

    @Environment(AppEnvironment.self) private var environment
    @State private var showsInvite = false

    private var canInvite: Bool { !CabalClosedModel.isReadOnly(cabal.status) }

    private var footer: String {
        let names = (cabal.admins ?? []).map { admin in
            admin.displayName.isEmpty ? (admin.handle.map { "@\($0)" } ?? "Someone") : admin.displayName
        }
        return CabalMemberCopy.membersFooter(admins: names, cabal: CabalHeaderCopy.shortName(cabal.name))
    }

    var body: some View {
        let viewerID = environment.viewer?.userID
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            CabalMembersList(model: model) {
                MonacoCard(style: .inset) {
                    MonacoGroupedList {
                        ForEach(model.preview) { entry in
                            CabalMemberLink(
                                entry: entry, viewerID: viewerID, subtitle: CabalMemberCopy.tabLine(entry.row),
                                chevron: true, isLast: entry.id == model.preview.last?.id,
                                hint: CabalMemberCopy.rightCallHint)
                        }
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
            }
            MonacoCard(style: .inset) {
                VStack(spacing: 0) {
                    NavigationLink(value: AnyAppRoute(MembersRoute(cabalID: cabal.id))) {
                        CabalCardRow(
                            symbol: "person.2", tone: MonacoTheme.brand,
                            title: CabalMemberCopy.seeAll(cabal.memberCount),
                            titleTone: MonacoTheme.brand, detail: nil, isLast: !canInvite,
                            trailing: { MonacoRowChevron() })
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-members-see-all")
                    if canInvite {
                        Button {
                            showsInvite = true
                        } label: {
                            CabalCardRow(
                                symbol: "person.badge.plus", tone: MonacoTheme.brand, title: CabalMemberCopy.invite,
                                titleTone: MonacoTheme.brand, detail: nil, isLast: true, trailing: { EmptyView() })
                        }
                        .buttonStyle(.monacoRow)
                        .accessibilityIdentifier("cabal-members-invite")
                    }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            Text(footer)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cabal-members-footer")
        }
        .sheet(isPresented: $showsInvite) { CabalInviteSheet(cabal: cabal) }
    }
}
