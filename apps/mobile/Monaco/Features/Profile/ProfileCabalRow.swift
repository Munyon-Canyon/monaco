import MonacoCore
import SwiftUI

enum ProfileCabalRowTrailing: Equatable {
    case figures(value: String, returnText: String?, tone: ProfileReturnTone?)
    case text(String)
    case none
}

struct ProfileCabalRowView: View {
    let cabalID: String
    let name: String
    let pictureURL: String?
    let subtitle: String
    let trailing: ProfileCabalRowTrailing
    let identifier: String
    let isLast: Bool

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init(row: ProfileCabalRow.Member, isLast: Bool) {
        cabalID = row.id
        name = row.name
        pictureURL = row.pictureURL
        subtitle = row.subtitle
        trailing = row.value.map { .figures(value: $0, returnText: row.returnText, tone: row.returnTone) } ?? .none
        identifier = "profile-cabal-row-\(row.id)"
        self.isLast = isLast
    }

    init(userCabal row: UserCabalRow, isLast: Bool) {
        cabalID = row.id
        name = row.name
        pictureURL = row.pictureURL
        subtitle = row.subtitle
        trailing = row.ownership.map { .text($0) } ?? .none
        identifier = "user-profile-cabal-row-\(row.id)"
        self.isLast = isLast
    }

    init(invite: ProfileCabalRow.Invited, isLast: Bool) {
        cabalID = invite.cabalID
        name = invite.name
        pictureURL = invite.pictureURL
        subtitle = invite.subtitle
        trailing = .text("Invited")
        identifier = "profile-invite-row-\(invite.cabalID)"
        self.isLast = isLast
    }

    var body: some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: cabalID))) {
            HStack(alignment: dynamicTypeSize.isAccessibilitySize ? .top : .center, spacing: MonacoTheme.Space.sm) {
                CabalMark(groupId: cabalID, name: name, size: .header, pictureUrl: pictureURL)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(name)
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .fixedSize(horizontal: false, vertical: true)
                    Text(subtitle)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize(horizontal: false, vertical: true)
                    if dynamicTypeSize.isAccessibilitySize {
                        trailingView
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                if !dynamicTypeSize.isAccessibilitySize {
                    trailingView
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.s)
            .frame(minHeight: 44)
            .contentShape(Rectangle())
            .overlay(alignment: .bottom) {
                if !isLast {
                    MonacoRule()
                        .padding(.leading, MonacoTheme.Space.sm + CabalMark.Size.header.rawValue)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(identifier)
    }

    @ViewBuilder private var trailingView: some View {
        switch trailing {
        case .figures(let value, let returnText, let tone):
            VStack(alignment: dynamicTypeSize.isAccessibilitySize ? .leading : .trailing, spacing: MonacoTheme.Space.xs)
            {
                Text(value)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.primaryText)
                if let returnText {
                    Text(returnText)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(Self.color(tone))
                }
            }
        case .text(let text):
            Text(text)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
        case .none:
            EmptyView()
        }
    }

    private static func color(_ tone: ProfileReturnTone?) -> Color {
        switch tone {
        case .gain: PnLTone.profit.color
        case .loss: PnLTone.loss.color
        case .flat, nil: PnLTone.flat.color
        }
    }
}
