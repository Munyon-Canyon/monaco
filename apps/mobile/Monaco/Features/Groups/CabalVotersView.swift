import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalVotersCopy {
    static let screenTitle = "Who votes"
    static let everyone = "Everyone"
    static let everyoneCaption = "Every member votes on each proposal."
    static let pick = "Pick voters"
    static let pickCaption = "Only the people you pick vote."
    static let votersHeader = "Voters"
    static let alwaysVotes = "Always votes"
    static let save = "Save"
    static let saving = "Saving…"
    static let saved = "Voters updated."

    static var auditedStrings: [String] {
        [
            screenTitle, everyone, everyoneCaption, pick, pickCaption, votersHeader, alwaysVotes, save, saving,
            saved,
        ]
    }
}

struct CabalVotersView: View {
    let model: CabalEditModel

    @State private var picksVoters: Bool
    @State private var picked: Set<String>
    @State private var toast: MonacoToast?

    init(model: CabalEditModel) {
        self.model = model
        switch model.voterChoice {
        case .list(let ids):
            _picksVoters = State(initialValue: true)
            _picked = State(initialValue: ids)
        case .everyone, nil:
            _picksVoters = State(initialValue: false)
            _picked = State(initialValue: [])
        }
    }

    private var cabal: Components.Schemas.Cabal? { model.cabal }

    private var creatorID: String? { cabal?.creator.userId }

    private var choice: CabalVoterChoice {
        guard picksVoters, let cabal else { return .everyone }
        let members = Set(cabal.members.map(\.userId))
        return .list(picked.intersection(members).union([cabal.creator.userId]))
    }

    private var canSave: Bool {
        guard !model.isSaving, let creatorID, let saved = model.voterChoice else { return false }
        return choice.patch(creatorID: creatorID) != saved.patch(creatorID: creatorID)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                MonacoGroupedList {
                    modeRow(
                        title: CabalVotersCopy.everyone, caption: CabalVotersCopy.everyoneCaption,
                        isSelected: !picksVoters, identifier: "voters-everyone"
                    ) { picksVoters = false }
                    modeRow(
                        title: CabalVotersCopy.pick, caption: CabalVotersCopy.pickCaption,
                        isSelected: picksVoters, identifier: "voters-pick", isLast: true
                    ) { picksVoters = true }
                }
                if picksVoters, let cabal {
                    memberSection(cabal)
                }
            }
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl)
            .disabled(model.isSaving)
        }
        .monacoCanvas()
        .navigationTitle(CabalVotersCopy.screenTitle)
        .navigationBarTitleDisplayMode(.inline)
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button(model.isSaving ? CabalVotersCopy.saving : CabalVotersCopy.save) {
                    Task { await save() }
                }
                .buttonStyle(.monacoPrimary)
                .disabled(!canSave)
                .accessibilityIdentifier("voters-save")
            }
        }
        .monacoToast($toast, placement: .aboveBottomCTA)
    }

    private func modeRow(
        title: String,
        caption: String,
        isSelected: Bool,
        identifier: String,
        isLast: Bool = false,
        select: @escaping () -> Void
    ) -> some View {
        Button {
            guard !isSelected else { return }
            Haptics.selection()
            select()
        } label: {
            MonacoRow(
                title: title, subtitle: caption, isLast: isLast,
                leading: {
                    Image(systemName: isSelected ? "largecircle.fill.circle" : "circle")
                        .font(.title3)
                        .foregroundStyle(isSelected ? MonacoTheme.brandFill : MonacoTheme.tertiaryText)
                }
            )
        }
        .buttonStyle(MonacoRowButtonStyle())
        .accessibilityAddTraits(isSelected ? [.isSelected] : [])
        .accessibilityIdentifier(identifier)
    }

    private func memberSection(_ cabal: Components.Schemas.Cabal) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(CabalVotersCopy.votersHeader)
                .padding(.horizontal, MonacoTheme.Space.m)
            MonacoGroupedList {
                ForEach(cabal.members, id: \.userId) { member in
                    memberRow(member, isLast: member.userId == cabal.members.last?.userId)
                }
            }
        }
    }

    private func memberRow(_ member: Components.Schemas.CabalMember, isLast: Bool) -> some View {
        let isCreator = member.userId == creatorID
        let isPicked = isCreator || picked.contains(member.userId)
        let subtitle = [member.handle.map { "@\($0)" }, isCreator ? CabalVotersCopy.alwaysVotes : nil]
            .compactMap { $0 }
            .joined(separator: " · ")
        return Button {
            guard !isCreator else { return }
            Haptics.selection()
            if isPicked {
                picked.remove(member.userId)
            } else {
                picked.insert(member.userId)
            }
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

    private func save() async {
        switch await model.saveVoters(choice) {
        case .saved:
            toast = MonacoToast(message: CabalVotersCopy.saved, isSuccess: true)
        case .unchanged:
            break
        case .failed(let message):
            toast = MonacoToast(message: message)
        }
    }
}
