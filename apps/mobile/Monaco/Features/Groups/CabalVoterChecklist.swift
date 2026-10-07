import MonacoAPI
import MonacoCore
import SwiftUI

struct CabalVoterChecklist: View {
    let cabal: Components.Schemas.Cabal
    @Binding var voters: CabalVoterChoice
    let isDisabled: Bool

    private var picked: Set<String> {
        if case .list(let ids) = voters { return ids }
        return []
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(EditCabalCopy.votersHeader)
                .padding(.horizontal, MonacoTheme.Space.m)
            MonacoGroupedList {
                ForEach(cabal.members, id: \.userId) { member in
                    memberRow(member, isLast: member.userId == cabal.members.last?.userId)
                }
            }
        }
        .disabled(isDisabled)
        .accessibilityIdentifier("edit-cabal-voters")
    }

    private func memberRow(_ member: Components.Schemas.CabalMember, isLast: Bool) -> some View {
        let isCreator = member.userId == cabal.creator.userId
        let isPicked = isCreator || picked.contains(member.userId)
        let subtitle = [member.handle.map { "@\($0)" }, isCreator ? EditCabalCopy.alwaysVotes : nil]
            .compactMap { $0 }
            .joined(separator: " · ")
        return Button {
            guard !isCreator else { return }
            Haptics.selection()
            var ids = picked
            if isPicked { ids.remove(member.userId) } else { ids.insert(member.userId) }
            voters = .list(ids)
        } label: {
            MonacoRow(
                title: member.shownName, subtitle: subtitle, isLast: isLast,
                leading: {
                    MonacoAvatar(
                        photoURL: member.photoUrl, displayName: member.shownName, size: 40, seed: member.userId)
                },
                trailing: {
                    if isPicked {
                        Image(systemName: "checkmark")
                            .font(MonacoTheme.Typo.calloutStrong)
                            .foregroundStyle(isCreator ? MonacoTheme.muted : MonacoTheme.brandFill)
                    }
                }
            )
        }
        .buttonStyle(MonacoRowButtonStyle())
        .accessibilityAddTraits(isPicked ? [.isSelected] : [])
        .accessibilityIdentifier("voters-member-\(member.userId)")
    }
}
