import MonacoCore
import SwiftUI

enum ProfileHeaderSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileHeader()
    }
}

struct ProfileHeaderPresets {
    var nameDraft: String?
    var showsEditProfile = false
    var showsFacePicker = false
    var saveName: (any DisplayNameSaving)?
    var savesOnOpen = false
}

extension EnvironmentValues {
    @Entry var profileHeaderPresets = ProfileHeaderPresets()
}

struct ProfileHeader: View {
    @EnvironmentObject private var auth: PrivyAuthService
    @Environment(AppSessionStore.self) private var session
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.profileHeaderPresets) private var presets

    @State private var showEditProfile = false

    private static let pencilTarget: CGFloat = 44

    private var displayName: String {
        let name = session.profile?.displayName.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return name.isEmpty ? "Member" : name
    }

    private var memberSince: String {
        guard let createdAt = session.profile?.createdAt else { return "Your profile" }
        return MemberSinceFormatter.format(createdAt)
    }

    var body: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            if session.profile != nil {
                identity
            } else if session.isLoading {
                skeleton
            } else {
                loadFailure
            }
        }
        .padding(.top, MonacoTheme.Space.m)
        .task {
            refresh?.register("profile-header") { [session, auth] in
                await session.reloadProfile(auth: auth)
            }
        }
        .sheet(isPresented: $showEditProfile) {
            NavigationStack {
                ScrollView {
                    ProfileNameEditor(
                        auth: auth,
                        initialDraft: presets.nameDraft,
                        saveName: presets.saveName,
                        savesOnOpen: presets.savesOnOpen
                    ) {
                        showEditProfile = false
                        toasts.show(success: "Name updated.")
                    }
                }
                .monacoSheet(title: "Edit name")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Cancel") { showEditProfile = false }
                    }
                }
            }
            .presentationDetents([.medium])
        }
        .onAppear {
            if presets.showsEditProfile { showEditProfile = true }
        }
    }

    private var skeleton: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            SkeletonBlock(width: 96, height: 96, radius: 48)
            SkeletonBlock(width: 168, height: 28)
            SkeletonBlock(width: 96, height: 16)
                .frame(minHeight: 44)
            SkeletonBlock(width: 112, height: 14)
        }
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading your profile")
        .accessibilityIdentifier("profile-header-loading")
    }

    private var loadFailure: some View {
        MonacoErrorRow(thing: "your profile", identifier: "profile-header-error") {
            Task { await session.reloadProfile(auth: auth) }
        }
    }

    private var identity: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            ProfilePhotoPicker(auth: auth, size: 96, initiallyOpen: presets.showsFacePicker) {
                toasts.current = $0
            }

            Text(displayName)
                .font(MonacoTheme.Typo.display)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(2)
                .minimumScaleFactor(0.7)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("profile-display-name")
                .overlay(alignment: .trailing) {
                    Button {
                        showEditProfile = true
                    } label: {
                        Image(systemName: "pencil")
                            .font(MonacoTheme.Typo.captionStrong)
                            .foregroundStyle(MonacoTheme.muted)
                            .frame(width: Self.pencilTarget, height: Self.pencilTarget)
                    }
                    .accessibilityLabel("Edit name")
                    .accessibilityIdentifier("profile-edit-button")
                    .offset(x: Self.pencilTarget)
                }
                .padding(.horizontal, Self.pencilTarget)

            if let handle = session.profile?.handle {
                NavigationLink(value: AnyAppRoute(HandleEditRoute())) {
                    HStack(spacing: MonacoTheme.Space.xs) {
                        Text("@\(handle)")
                            .font(MonacoTheme.Typo.rowTitle)
                            .foregroundStyle(MonacoTheme.brand)
                            .lineLimit(1)
                            .minimumScaleFactor(0.7)
                        Image(systemName: "pencil")
                            .font(MonacoTheme.Typo.captionStrong)
                            .foregroundStyle(MonacoTheme.muted)
                            .accessibilityHidden(true)
                    }
                    .frame(minHeight: 44)
                }
                .buttonStyle(.plain)
                .accessibilityHint("Edit your handle")
                .accessibilityIdentifier("profile-handle")
            }

            Text(memberSince)
                .font(MonacoTheme.Typo.callout)
                .foregroundStyle(MonacoTheme.tertiaryText)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("profile-member-since")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("profile-header")
    }
}
