import MonacoCore
import SwiftUI

enum CabalsUpNextSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        CabalsUpNextSection()
    }
}

enum CabalsUpNextMetrics {
    static let mark: CGFloat = 40
}

struct CabalsUpNextSection: View {
    @Environment(CabalsTabStore.self) private var store
    @State private var inviting: CabalTile?

    private var tiles: [CabalTile] {
        guard store.votes.shown == nil else { return [] }
        return store.content?.upNext ?? []
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if !tiles.isEmpty {
                MonacoSectionHeader("Up next")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoCard(style: .inset) {
                    VStack(spacing: 0) {
                        ForEach(Array(tiles.enumerated()), id: \.element.id) { index, tile in
                            if index > 0 {
                                MonacoRule()
                                    .padding(.leading, CabalsUpNextMetrics.mark + MonacoTheme.Space.sm)
                            }
                            CabalsUpNextRow(tile: tile) { inviting = $0 }
                        }
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.m)
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("cabals-upnext")
                .cabalInviteSheet($inviting)
            }
        }
    }
}

private struct CabalsUpNextRow: View {
    let tile: CabalTile
    let invite: (CabalTile) -> Void

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var sentence: String { tile.nextStepLine ?? "" }
    private var title: String { tile.nextStep == .invite ? "Invite" : "Propose" }
    private var kind: CabalMenuAction.Kind { tile.nextStep == .invite ? .invite : .propose }
    private var icon: String { (tile.nextStep == .invite ? MonacoIcon.invite : MonacoIcon.propose).symbol }

    var body: some View {
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    summary
                    capsule
                }
            } else {
                HStack(spacing: MonacoTheme.Space.sm) {
                    summary
                    capsule
                }
            }
        }
        .padding(.vertical, MonacoTheme.Space.sm)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-upnext-row-\(tile.id)")
    }

    private var summary: some View {
        Button {
            Haptics.selection()
            environment.navigator.open(CabalRoute(id: tile.id), in: .cabals)
        } label: {
            HStack(spacing: MonacoTheme.Space.sm) {
                CabalMark(
                    groupId: tile.id, name: tile.name, size: CabalsUpNextMetrics.mark, pictureUrl: tile.pictureURL)
                VStack(alignment: .leading, spacing: 0) {
                    Text(tile.name)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(sentence)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .multilineTextAlignment(.leading)
                Spacer(minLength: 0)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(tile.name), \(sentence)")
        .accessibilityAddTraits(.isButton)
        .accessibilityAction(named: title) { perform() }
    }

    private var capsule: some View {
        Button {
            perform()
        } label: {
            Label(title, systemImage: icon)
        }
        .buttonStyle(.monacoCompact)
        .accessibilityIdentifier("cabals-upnext-action-\(tile.id)")
    }

    private func perform() {
        CabalsTabActions.perform(kind, on: tile, navigator: environment.navigator, invite: invite)
    }
}
