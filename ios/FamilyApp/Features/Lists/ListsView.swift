import SwiftUI

/// All shared lists of the household. On wide screens (iPad), the lists and
/// the items of the selected list are side by side.
struct ListsView: View {
    @Environment(AppSession.self) private var session
    @Environment(\.horizontalSizeClass) private var sizeClass
    @State private var lists: [SharedList] = []
    @State private var hasLoaded = false
    @State private var editing: ListEditor.Target?
    /// The list shown in the second column on wide screens.
    @State private var selectedListID: SharedList.ID?

    var body: some View {
        if sizeClass == .regular {
            NavigationSplitView {
                decorated(
                    List(selection: $selectedListID) {
                        rows(linked: false)
                    }
                )
            } detail: {
                if let list = lists.first(where: { $0.id == selectedListID }) {
                    NavigationStack {
                        // The counts in the first column follow the changes.
                        ListDetailView(list: list) { await load() }
                    }
                    .id(list.id)
                } else {
                    ContentUnavailableView("Select a list", systemImage: "checklist")
                }
            }
            .onChange(of: lists) { _, newLists in
                // Show the first list rather than an empty column.
                if selectedListID == nil || !newLists.contains(where: { $0.id == selectedListID }) {
                    selectedListID = newLists.first?.id
                }
            }
        } else {
            NavigationStack {
                decorated(
                    List {
                        rows(linked: true)
                    }
                )
                .navigationDestination(for: SharedList.self) { list in
                    ListDetailView(list: list)
                }
            }
        }
    }

    /// One row per list: a link on iPhone, a selectable row on iPad.
    @ViewBuilder
    private func rows(linked: Bool) -> some View {
        ForEach(lists) { list in
            Group {
                if linked {
                    NavigationLink(value: list) { ListRow(list: list) }
                } else {
                    ListRow(list: list).tag(list.id)
                }
            }
            .swipeActions(edge: .leading) {
                Button("Edit", systemImage: "pencil") { editing = .edit(list) }
                    .tint(.orange)
            }
        }
        .onDelete(perform: delete)
    }

    /// Title, toolbar, sheets and loading, shared by both layouts.
    private func decorated(_ content: some View) -> some View {
        content
            .navigationTitle("Lists")
            .overlay {
                if hasLoaded && lists.isEmpty {
                    ContentUnavailableView {
                        Label("No lists yet", systemImage: "checklist")
                    } description: {
                        Text("Create a list for groceries, to-dos or anything you share.")
                    } actions: {
                        Button("New list") { editing = .create }
                            .buttonStyle(.borderedProminent)
                    }
                }
            }
            .toolbar {
                ToolbarItem(placement: .primaryAction) {
                    Button("New list", systemImage: "plus") { editing = .create }
                        .accessibilityIdentifier("toolbar.add")
                        .keyboardShortcut("n", modifiers: .command)
                }
                ToolbarItem(placement: .secondaryAction) {
                    WideRefreshButton { await load() }
                }
            }
            .sheet(item: $editing) { target in
                ListEditor(target: target) { await load() }
            }
            .refreshable { await load() }
            .task { await load() }
            .onFamilyDataChanged { await load() }
    }

    private func load() async {
        await session.run { api in
            lists = try await api.get("lists")
            hasLoaded = true
        }
    }

    private func delete(at offsets: IndexSet) {
        let removed = offsets.map { lists[$0] }
        lists.remove(atOffsets: offsets)
        Task {
            for list in removed {
                await session.run { api in try await api.delete("lists/\(list.id)") }
            }
            await load()
        }
    }
}

private struct ListRow: View {
    let list: SharedList

    var body: some View {
        HStack(spacing: 12) {
            Text(list.displayEmoji)
                .font(.title2)
            VStack(alignment: .leading, spacing: 2) {
                Text(list.name)
                    .font(.headline)
                Group {
                    if list.totalCount == 0 {
                        Text("Empty")
                    } else if list.openCount == 0 {
                        Text("All done")
                    } else {
                        Text("\(list.openCount) left")
                    }
                }
                .font(.subheadline)
                .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 4)
    }
}

/// Creates or renames a list.
struct ListEditor: View {
    enum Target: Identifiable {
        case create
        case edit(SharedList)

        var id: String {
            switch self {
            case .create: "new"
            case .edit(let list): list.id
            }
        }
    }

    let target: Target
    let onSave: () async -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var kind = ListKind.grocery
    @State private var emoji = ""
    @State private var isSaving = false

    var body: some View {
        NavigationStack {
            Form {
                TextField("Name", text: $name)
                    .accessibilityIdentifier("editor.name")
                Picker("Type", selection: $kind) {
                    ForEach(ListKind.allCases) { kind in
                        Text(kind.title).tag(kind)
                    }
                }
                TextField("Emoji (optional)", text: $emoji)
            }
            .navigationTitle(isNew ? "New list" : "Edit list")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save", action: save)
                        .accessibilityIdentifier("editor.save")
                        .disabled(name.trimmingCharacters(in: .whitespaces).isEmpty || isSaving)
                }
            }
            .onAppear {
                if case .edit(let list) = target {
                    name = list.name
                    kind = list.kind
                    emoji = list.emoji
                }
            }
        }
        .presentationDetents([.medium])
    }

    private var isNew: Bool {
        if case .create = target { return true }
        return false
    }

    private func save() {
        isSaving = true
        let body: JSONBody = ["name": .string(name), "kind": .string(kind.rawValue), "emoji": .string(emoji)]
        Task {
            let saved = await session.run { api in
                switch target {
                case .create:
                    let _: SharedList = try await api.post("lists", body)
                case .edit(let list):
                    let _: SharedList = try await api.patch("lists/\(list.id)", body)
                }
            }
            isSaving = false
            if saved {
                await onSave()
                dismiss()
            }
        }
    }
}
