import SwiftUI

/// Login, joining a household with an invite code, or creating the very first
/// account with the setup code from the API logs.
struct AuthView: View {
    @Environment(AppSession.self) private var session

    private enum Mode: Hashable {
        case login, join, setup
    }

    @State private var mode = Mode.login
    @State private var server = ""
    @State private var username = ""
    @State private var password = ""
    @State private var displayName = ""
    @State private var code = ""
    @State private var householdName = ""
    @State private var isWorking = false
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    // Verbatim: a localized string turns the URL into a blue Markdown link.
                    TextField("Server", text: $server, prompt: Text(verbatim: "https://family.example.com"))
                        .keyboardType(.URL)
                        .textContentType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                } header: {
                    Text("Server")
                } footer: {
                    Text("The address of your Family API.")
                }

                Section {
                    Picker("Mode", selection: $mode) {
                        Text("Log in").tag(Mode.login)
                        Text("Join").tag(Mode.join)
                        Text("First setup").tag(Mode.setup)
                    }
                    .pickerStyle(.segmented)
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                }

                Section {
                    TextField("Username", text: $username)
                        .textContentType(.username)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    SecureField("Password", text: $password)
                        .textContentType(mode == .login ? .password : .newPassword)
                    if mode != .login {
                        TextField("Your first name", text: $displayName)
                            .textContentType(.givenName)
                    }
                } footer: {
                    if mode != .login {
                        Text("At least 8 characters for the password.")
                    }
                }

                if mode == .join {
                    Section {
                        TextField("Invite code", text: $code)
                            .textInputAutocapitalization(.characters)
                            .autocorrectionDisabled()
                    } footer: {
                        Text("Ask a member of the household to create an invite in Settings.")
                    }
                }

                if mode == .setup {
                    Section {
                        TextField("Setup code", text: $code)
                            .textInputAutocapitalization(.characters)
                            .autocorrectionDisabled()
                        TextField("Household name", text: $householdName)
                    } footer: {
                        Text("Only for the very first account. The setup code is in the API logs.")
                    }
                }

                if let errorMessage {
                    Section {
                        Label(errorMessage, systemImage: "exclamationmark.triangle")
                            .foregroundStyle(.red)
                    }
                }

                Section {
                    Button(action: submit) {
                        HStack {
                            Spacer()
                            if isWorking {
                                ProgressView()
                            } else {
                                Text(submitTitle).bold()
                            }
                            Spacer()
                        }
                    }
                    .disabled(!canSubmit || isWorking)
                }
            }
            .navigationTitle("Welcome")
            .onAppear {
                if server.isEmpty, let saved = session.serverURL {
                    server = saved.absoluteString
                }
            }
        }
    }

    private var submitTitle: LocalizedStringKey {
        mode == .login ? "Log in" : "Create my account"
    }

    private var canSubmit: Bool {
        let base = !server.isEmpty && !username.isEmpty && !password.isEmpty
        switch mode {
        case .login: return base
        case .join, .setup: return base && !displayName.isEmpty && !code.isEmpty
        }
    }

    private func submit() {
        isWorking = true
        errorMessage = nil
        Task {
            defer { isWorking = false }
            do {
                switch mode {
                case .login:
                    try await session.login(server: server, username: username, password: password)
                case .join:
                    try await session.register(
                        server: server, username: username, password: password,
                        displayName: displayName, signup: .invite(code: code))
                case .setup:
                    try await session.register(
                        server: server, username: username, password: password,
                        displayName: displayName, signup: .setup(code: code, householdName: householdName))
                }
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }
}
