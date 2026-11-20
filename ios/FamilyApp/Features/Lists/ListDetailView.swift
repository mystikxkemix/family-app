import SwiftUI

/// The items of one list: add quickly, tap to check, swipe to delete.
struct ListDetailView: View {
    let list: SharedList

    @Environment(AppSession.self) private var session
    @State private var items: [ListItem] = []
    @State private var newItemText = ""
    @State private var editingItem: ListItem?
    @FocusState private var isAddFieldFocused: Bool

    private var openItems: [ListItem] { items.filter { !$0.checked } }
    private var checkedItems: [ListItem] {
        items.filter(\.checked).sorted { ($0.checkedAt ?? .distantPast) > ($1.checkedAt ?? .distantPast) }
    }

    var body: some View {
        List {
            Section {
                HStack {
                    Image(systemName: "plus.circle.fill")
                        .foregroundStyle(.tint)
                    TextField("Add an item", text: $newItemText)
                        .accessibilityIdentifier("list.addItem")
                        .focused($isAddFieldFocused)
                        .submitLabel(.done)
                        .onSubmit(addItem)
                }
                ForEach(openItems) { item in
                    ItemRow(item: item, toggle: { toggle(item) })
                        .swipeActions { deleteButton(item) }
                        .contextMenu { Button("Edit", systemImage: "pencil") { editingItem = item } }
                }
            }

            if !checkedItems.isEmpty {
                Section {
                    ForEach(checkedItems) { item in
                        ItemRow(item: item, toggle: { toggle(item) })
                            .swipeActions { deleteButton(item) }
                    }
                } header: {
                    Text(list.kind == .grocery ? LocalizedStringKey("In the cart") : LocalizedStringKey("Done"))
                }
            }
        }
        .navigationTitle(list.displayEmoji + " " + list.name)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            Menu("More", systemImage: "ellipsis.circle") {
                Button("Remove checked items", systemImage: "trash", role: .destructive, action: clearChecked)
                    .disabled(checkedItems.isEmpty)
            }
        }
        .sheet(item: $editingItem) { item in
            ItemEditor(item: item) { await load() }
        }
        .refreshable { await load() }
        .task { await load() }
        .onFamilyDataChanged { await load() }
    }

    private func deleteButton(_ item: ListItem) -> some View {
        Button("Delete", systemImage: "trash", role: .destructive) {
            items.removeAll { $0.id == item.id }
            Task { await session.run { api in try await api.delete("lists/\(list.id)/items/\(item.id)") } }
        }
    }

    private func load() async {
        await session.run { api in
            items = try await api.get("lists/\(list.id)/items")
        }
    }

    /// Adds the item and keeps the keyboard open for the next one.
    private func addItem() {
        let text = newItemText.trimmingCharacters(in: .whitespaces)
        guard !text.isEmpty else { return }
        newItemText = ""
        isAddFieldFocused = true
        Task {
            await session.run { api in
                let item: ListItem = try await api.post("lists/\(list.id)/items", ["text": .string(text)])
                items.append(item)
            }
        }
    }

    /// Checks or unchecks at once, then saves. A failure reloads the truth.
    private func toggle(_ item: ListItem) {
        guard let index = items.firstIndex(where: { $0.id == item.id }) else { return }
        withAnimation {
            items[index].checked.toggle()
            items[index].checkedAt = items[index].checked ? .now : nil
        }
        let checked = items[index].checked
        Task {
            let saved = await session.run { api in
                let updated: ListItem = try await api.patch("lists/\(list.id)/items/\(item.id)", ["checked": .bool(checked)])
                if let index = items.firstIndex(where: { $0.id == updated.id }) {
                    items[index] = updated
                }
            }
            if !saved { await load() }
        }
    }

    private func clearChecked() {
        withAnimation { items.removeAll(where: \.checked) }
        Task {
            await session.run { api in
                let _: [String: Int] = try await api.post("lists/\(list.id)/clear-checked")
            }
        }
    }
}

private struct ItemRow: View {
    let item: ListItem
    let toggle: () -> Void

    var body: some View {
        Button(action: toggle) {
            HStack(spacing: 12) {
                Image(systemName: item.checked ? "checkmark.circle.fill" : "circle")
                    .font(.title3)
                    .foregroundStyle(item.checked ? Color.secondary : Color.accentColor)
                VStack(alignment: .leading, spacing: 2) {
                    Text(item.text)
                        .strikethrough(item.checked)
                        .foregroundStyle(item.checked ? .secondary : .primary)
                    if !item.note.isEmpty {
                        Text(item.note)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
        .buttonStyle(.plain)
    }
}

/// Edits the text and note of an item.
private struct ItemEditor: View {
    let item: ListItem
    let onSave: () async -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @State private var note = ""

    var body: some View {
        NavigationStack {
            Form {
                TextField("Item", text: $text)
                TextField("Note (quantity, brand...)", text: $note, axis: .vertical)
            }
            .navigationTitle("Edit item")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        Task {
                            let saved = await session.run { api in
                                let _: ListItem = try await api.patch(
                                    "lists/\(item.listId)/items/\(item.id)",
                                    ["text": .string(text), "note": .string(note)])
                            }
                            if saved {
                                await onSave()
                                dismiss()
                            }
                        }
                    }
                    .disabled(text.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
            .onAppear {
                text = item.text
                note = item.note
            }
        }
        .presentationDetents([.medium])
    }
}
