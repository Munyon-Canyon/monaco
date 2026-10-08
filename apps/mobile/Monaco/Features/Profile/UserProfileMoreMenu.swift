import SwiftUI

struct UserProfileMoreMenu: View {
    enum SafetyAction: CaseIterable {
        case report
        case block

        var title: String {
            switch self {
            case .report: "Report"
            case .block: "Block"
            }
        }

        var systemImage: String {
            switch self {
            case .report: "exclamationmark.bubble"
            case .block: "nosign"
            }
        }

        var role: ButtonRole? { .destructive }
    }

    var body: some View {
        Menu {
            Section("Report and block open soon.") {
                ForEach(SafetyAction.allCases, id: \.self) { action in
                    Button(role: action.role) {
                    } label: {
                        Label(action.title, systemImage: action.systemImage)
                    }
                    .foregroundStyle(MonacoTheme.destructive)
                    .disabled(true)
                }
            }
        } label: {
            Image(systemName: "ellipsis")
                .frame(minWidth: 44, minHeight: 44)
        }
        .accessibilityLabel("More")
        .accessibilityIdentifier("user-profile-more")
    }
}
