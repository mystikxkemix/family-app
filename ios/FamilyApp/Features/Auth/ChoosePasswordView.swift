import SwiftUI

/// Shown instead of the app while the account must choose a new password:
/// the first (admin) account comes with a password from the server settings.
struct ChoosePasswordView: View {
    @Environment(AppSession.self) private var session
    @State private var current = ""
    @State private var new = ""
    @State private var confirmation = ""
    @State private var isSaving = false

    private var isValid: Bool {
        !current.isEmpty && new.count >= 8 && new == confirmation && new != current
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Label("For security, choose your own password before you start.", systemImage: "lock.shield")
                }
                if session.loginPassword == nil {
                    Section {
                        SecureField("Current password", text: $current)
                            .autoFillContentType(.password)
                            .accessibilityIdentifier("password.current")
                    }
                }
                Section {
                    SecureField("New password", text: $new)
                        .autoFillContentType(.newPassword)
                        .accessibilityIdentifier("password.new")
                    SecureField("Confirm new password", text: $confirmation)
                        .autoFillContentType(.newPassword)
                        .accessibilityIdentifier("password.confirm")
                } footer: {
                    if !new.isEmpty && !confirmation.isEmpty && new != confirmation {
                        Text("The two passwords are not the same.")
                            .foregroundStyle(.red)
                    } else {
                        Text("At least 8 characters, different from the current one.")
                    }
                }
                Section {
                    Button(action: save) {
                        HStack {
                            Spacer()
                            if isSaving { ProgressView() } else { Text("Save my password").bold() }
                            Spacer()
                        }
                    }
                    .disabled(!isValid || isSaving)
                    .accessibilityIdentifier("password.save")
                }
            }
            .navigationTitle("Choose your password")
            .toolbar {
                Button("Log out") {
                    Task { await session.logout() }
                }
            }
            .onAppear {
                if let loginPassword = session.loginPassword { current = loginPassword }
            }
        }
    }

    private func save() {
        isSaving = true
        Task {
            await session.changePassword(current: current, new: new)
            isSaving = false
        }
    }
}
