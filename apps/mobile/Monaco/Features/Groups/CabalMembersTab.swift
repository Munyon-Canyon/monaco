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

    var body: some View {
        let viewerID = environment.viewer?.userID
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            CabalMembersList(model: model) {
                MonacoGroupedList {
                    ForEach(model.preview) { entry in
                        CabalMemberLink(
                            entry: entry, viewerID: viewerID, subtitle: CabalMemberCopy.tabLine(entry.row),
                            chevron: true, isLast: entry.id == model.preview.last?.id)
                    }
                }
            }
            MonacoGroupedList {
                NavigationLink(value: AnyAppRoute(MembersRoute(cabalID: cabal.id))) {
                    MonacoRow(
                        title: CabalMemberCopy.seeAll(cabal.memberCount), chevron: true, isLast: !canInvite,
                        leading: { SunkenGlyphMark(systemImage: "person.2") }, trailing: { EmptyView() })
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("cabal-members-see-all")
                if canInvite {
                    Button {
                        showsInvite = true
                    } label: {
                        MonacoRow(
                            title: CabalMemberCopy.invite, titleColor: MonacoTheme.brand, isLast: true,
                            leading: { SunkenGlyphMark(systemImage: "person.badge.plus") }, trailing: { EmptyView() })
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-members-invite")
                }
            }
            .tintedCard()
            .padding(.horizontal, MonacoTheme.Space.gutter)
            Text(CabalMemberCopy.membersFooter)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cabal-members-footer")
        }
        .sheet(isPresented: $showsInvite) { CabalInviteSheet(cabal: cabal) }
    }
}

extension View {
    func tintedCard() -> some View {
        background(MonacoTheme.brandWash)
            .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
    }
}
