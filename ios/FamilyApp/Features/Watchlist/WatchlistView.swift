import SwiftUI

/// Movies and TV shows to watch together.
struct WatchlistView: View {
    @Environment(AppSession.self) private var session
    @State private var items: [Media] = []
    @State private var status = MediaStatus.toWatch
    @State private var hasLoaded = false
    @State private var editing: MediaEditor.Target?
    @State private var pick: Media?

    private var visibleItems: [Media] { items.filter { $0.status == status } }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Picker("Status", selection: $status) {
                        ForEach(MediaStatus.allCases) { status in
                            Text(status.title).tag(status)
                        }
                    }
                    .pickerStyle(.segmented)
                    .accessibilityIdentifier("status.picker")
                    .listRowBackground(Color.clear)
                    .listRowInsets(EdgeInsets())
                }

                ForEach(visibleItems) { media in
                    NavigationLink(value: media) {
                        MediaRow(media: media)
                    }
                        .swipeActions(edge: .leading) { statusActions(media) }
                        .swipeActions(edge: .trailing) {
                            Button("Delete", systemImage: "trash", role: .destructive) { delete(media) }
                            interestButton(media)
                        }
                }
            }
            .navigationTitle("Watchlist")
            .navigationDestination(for: Media.self) { media in
                MediaDetailView(media: media) { await load() }
            }
            .overlay {
                if hasLoaded && visibleItems.isEmpty {
                    ContentUnavailableView {
                        Label("Nothing here", systemImage: "popcorn")
                    } description: {
                        Text("Add a movie or a show you want to watch together.")
                    }
                }
            }
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Pick for us", systemImage: "dice", action: pickRandom)
                        .accessibilityIdentifier("watchlist.pick")
                        .disabled(items.allSatisfy { $0.status != .toWatch })
                }
                ToolbarItem(placement: .primaryAction) {
                    Button("Add", systemImage: "plus") { editing = .create }
                        .accessibilityIdentifier("toolbar.add")
                }
            }
            .sheet(item: $editing) { target in
                MediaEditor(target: target) { await load() }
            }
            .alert(
                "Tonight's pick",
                isPresented: Binding(get: { pick != nil }, set: { if !$0 { pick = nil } }),
                presenting: pick
            ) { media in
                Button("Let's watch it") { setStatus(media, .watching) }
                Button("Pick again", action: pickRandom)
                Button("Cancel", role: .cancel) {}
            } message: { media in
                Text(media.title)
            }
            .refreshable { await load() }
            .task { await load() }
            .onFamilyDataChanged { await load() }
        }
    }

    @ViewBuilder
    private func statusActions(_ media: Media) -> some View {
        switch media.status {
        case .toWatch:
            Button("Watching", systemImage: "play") { setStatus(media, .watching) }.tint(.blue)
            Button("Watched", systemImage: "checkmark") { setStatus(media, .watched) }.tint(.green)
        case .watching:
            Button("Watched", systemImage: "checkmark") { setStatus(media, .watched) }.tint(.green)
        case .watched:
            Button("To watch", systemImage: "arrow.uturn.backward") { setStatus(media, .toWatch) }.tint(.orange)
        }
    }

    private func interestButton(_ media: Media) -> some View {
        let interested = media.interestedUserIds.contains(session.me?.user.id ?? "")
        return Button(
            interested ? LocalizedStringKey("Not for me") : LocalizedStringKey("I want to watch it"),
            systemImage: interested ? "heart.slash" : "heart"
        ) {
            update(media, ["interested": .bool(!interested)])
        }
        .tint(.pink)
    }

    private func load() async {
        await session.run { api in
            items = try await api.get("watchlist")
            hasLoaded = true
        }
    }

    private func setStatus(_ media: Media, _ newStatus: MediaStatus) {
        update(media, ["status": .string(newStatus.rawValue)])
    }

    private func update(_ media: Media, _ body: JSONBody) {
        Task {
            await session.run { api in
                let updated: Media = try await api.patch("watchlist/\(media.id)", body)
                if let index = items.firstIndex(where: { $0.id == updated.id }) {
                    withAnimation { items[index] = updated }
                }
            }
        }
    }

    private func delete(_ media: Media) {
        items.removeAll { $0.id == media.id }
        Task { await session.run { api in try await api.delete("watchlist/\(media.id)") } }
    }

    /// Picks a random title to watch. Titles that everyone wants come first.
    private func pickRandom() {
        let candidates = items.filter { $0.status == .toWatch }
        let memberCount = max(session.me?.members.count ?? 1, 1)
        let everyoneWants = candidates.filter { $0.interestedUserIds.count >= memberCount }
        pick = (everyoneWants.isEmpty ? candidates : everyoneWants).randomElement()
    }
}

/// A small poster, or the movie/show symbol when there is none.
struct PosterView: View {
    let url: String?
    let kind: MediaKind

    var body: some View {
        Group {
            if let url, let imageURL = URL(string: url) {
                AsyncImage(url: imageURL) { image in
                    image.resizable().scaledToFill()
                } placeholder: {
                    Color.secondary.opacity(0.15)
                }
            } else {
                Image(systemName: kind.symbol)
                    .font(.title2)
                    .foregroundStyle(.tint)
            }
        }
        .frame(width: 44, height: 66)
        .clipShape(RoundedRectangle(cornerRadius: 6))
    }
}

private struct MediaRow: View {
    let media: Media

    var body: some View {
        HStack(spacing: 12) {
            PosterView(url: media.posterUrl, kind: media.kind)
            VStack(alignment: .leading, spacing: 2) {
                HStack(alignment: .firstTextBaseline) {
                    Text(media.title).font(.headline)
                    if let year = media.year, year > 0 {
                        Text(String(year)).foregroundStyle(.secondary)
                    }
                }
                if !media.note.isEmpty {
                    Text(media.note)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
                InterestLine(userIds: media.interestedUserIds)
                if media.downloadRequestedAt != nil {
                    Label("Download requested", systemImage: "arrow.down.circle")
                        .font(.caption)
                        .foregroundStyle(.green)
                }
            }
        }
        .padding(.vertical, 2)
    }
}

/// Adds or edits a movie or a show. When TMDB (or Radarr/Sonarr) is set up on
/// the server, a search fills the title, year, poster and ids. Downloads and
/// releases are on the details page.
struct MediaEditor: View {
    enum Target: Identifiable {
        case create
        case edit(Media)

        var id: String {
            switch self {
            case .create: "new"
            case .edit(let media): media.id
            }
        }
    }

    let target: Target
    let onSave: () async -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var kind = MediaKind.movie
    @State private var title = ""
    @State private var year = ""
    @State private var note = ""
    @State private var posterUrl: String?
    @State private var overview: String?
    @State private var externalIds = ExternalIDs()
    @State private var searchText = ""
    @State private var results: [MediaSearchResult] = []
    @State private var isSearching = false
    @State private var isSaving = false

    private var canSearch: Bool { session.me?.integrations.canSearch(kind) ?? false }

    var body: some View {
        NavigationStack {
            Form {
                Picker("Type", selection: $kind) {
                    ForEach(MediaKind.allCases) { kind in
                        Label { Text(kind.title) } icon: { Image(systemName: kind.symbol) }.tag(kind)
                    }
                }
                .pickerStyle(.segmented)
                .disabled(!isNew)

                if isNew && canSearch {
                    searchSection
                }

                Section {
                    HStack(alignment: .top) {
                        if posterUrl != nil {
                            PosterView(url: posterUrl, kind: kind)
                        }
                        VStack {
                            TextField("Title", text: $title)
                                .accessibilityIdentifier("editor.name")
                            TextField("Year (optional)", text: $year)
                                .keyboardType(.numberPad)
                        }
                    }
                    TextField("Note (who recommended it, where to watch...)", text: $note, axis: .vertical)
                }
            }
            .navigationTitle(isNew ? "Add to watchlist" : "Edit")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save", action: save)
                        .accessibilityIdentifier("editor.save")
                        .disabled(title.trimmingCharacters(in: .whitespaces).isEmpty || isSaving)
                }
            }
            .onAppear(perform: fill)
        }
        .presentationDetents([.large])
    }

    private var searchSection: some View {
        Section {
            HStack {
                TextField("Search a title", text: $searchText)
                    .submitLabel(.search)
                    .onSubmit(search)
                if isSearching {
                    ProgressView()
                } else {
                    Button("Search", systemImage: "magnifyingglass", action: search)
                        .labelStyle(.iconOnly)
                        .disabled(searchText.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
            ForEach(results) { result in
                Button {
                    choose(result)
                } label: {
                    SearchResultRow(result: result)
                }
            }
        } footer: {
            Text("Pick a result to fill the title and the poster, or type them yourself below.")
        }
    }

    private var isNew: Bool {
        if case .create = target { return true }
        return false
    }

    private func fill() {
        guard case .edit(let media) = target else { return }
        kind = media.kind
        title = media.title
        year = media.year.map(String.init) ?? ""
        note = media.note
        posterUrl = media.posterUrl
        overview = media.overview
        externalIds = media.externalIds
    }

    private func search() {
        let term = searchText.trimmingCharacters(in: .whitespaces)
        guard !term.isEmpty else { return }
        isSearching = true
        Task {
            await session.run { api in
                results = try await api.get(
                    "search/media",
                    query: [URLQueryItem(name: "kind", value: kind.rawValue), URLQueryItem(name: "q", value: term), AppLanguage.queryItem])
            }
            isSearching = false
        }
    }

    private func choose(_ result: MediaSearchResult) {
        title = result.title
        year = result.year.map(String.init) ?? ""
        posterUrl = result.posterUrl
        overview = result.overview
        externalIds = result.externalIds
        results = []
    }

    private func save() {
        isSaving = true
        var ids: [String: JSONValue] = [:]
        if let tmdb = externalIds.tmdb { ids["tmdb"] = .int(tmdb) }
        if let tvdb = externalIds.tvdb { ids["tvdb"] = .int(tvdb) }
        if let imdb = externalIds.imdb { ids["imdb"] = .string(imdb) }
        var body: JSONBody = [
            "title": .string(title),
            "year": .int(Int(year) ?? 0),
            "note": .string(note),
            "posterUrl": .string(posterUrl ?? ""),
            "overview": .string(overview ?? ""),
            "externalIds": .object(ids),
        ]
        if isNew { body["kind"] = .string(kind.rawValue) }
        Task {
            let saved = await session.run { api in
                switch target {
                case .create:
                    let _: Media = try await api.post("watchlist", body)
                case .edit(let media):
                    let _: Media = try await api.patch("watchlist/\(media.id)", body)
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

/// Prowlarr interactive search: pick a release and send it to the download client.
struct ReleasesView: View {
    let kind: MediaKind
    let initialQuery: String

    @Environment(AppSession.self) private var session
    @State private var query = ""
    @State private var releases: [Release] = []
    @State private var isSearching = false
    @State private var hasSearched = false
    @State private var selected: Release?
    @State private var grabbedMessage: String?

    var body: some View {
        List {
            Section {
                HStack {
                    TextField("Search", text: $query)
                        .submitLabel(.search)
                        .onSubmit { Task { await search() } }
                    if isSearching { ProgressView() }
                }
            }
            ForEach(releases) { release in
                Button { selected = release } label: { ReleaseRow(release: release) }
                    .foregroundStyle(.primary)
            }
        }
        .navigationTitle("Releases")
        .overlay {
            if hasSearched && !isSearching && releases.isEmpty {
                ContentUnavailableView.search(text: query)
            }
        }
        .confirmationDialog(
            "Download this release?",
            isPresented: Binding(get: { selected != nil }, set: { if !$0 { selected = nil } }),
            titleVisibility: .visible,
            presenting: selected
        ) { release in
            Button("Download") { grab(release) }
        } message: { release in
            Text(release.title)
        }
        .alert(
            "Download",
            isPresented: Binding(get: { grabbedMessage != nil }, set: { if !$0 { grabbedMessage = nil } }),
            actions: { Button("OK", role: .cancel) {} },
            message: { Text(grabbedMessage ?? "") }
        )
        .task {
            if query.isEmpty {
                query = initialQuery
                await search()
            }
        }
    }

    private func search() async {
        let term = query.trimmingCharacters(in: .whitespaces)
        guard !term.isEmpty else { return }
        isSearching = true
        await session.run { api in
            let found: [Release] = try await api.get(
                "releases",
                query: [URLQueryItem(name: "kind", value: kind.rawValue), URLQueryItem(name: "q", value: term)])
            // Most seeded first; usenet releases (no seeders) after.
            releases = found.sorted { ($0.seeders ?? -1) > ($1.seeders ?? -1) }
        }
        isSearching = false
        hasSearched = true
    }

    private func grab(_ release: Release) {
        Task {
            let sent = await session.run { api in
                let _: NoContent = try await api.post(
                    "releases/grab", ["guid": .string(release.guid), "indexerId": .int(release.indexerId)])
            }
            if sent {
                grabbedMessage = String(localized: "Sent. The download will start soon.")
            }
        }
    }
}

private struct ReleaseRow: View {
    let release: Release

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(release.title)
                .font(.subheadline)
                .lineLimit(3)
            HStack(spacing: 12) {
                Text(release.size.formatted(.byteCount(style: .file)))
                if let seeders = release.seeders {
                    Label { Text(seeders, format: .number) } icon: { Image(systemName: "arrow.up.circle") }
                }
                Text(release.publishDate, format: .relative(presentation: .named))
                if let indexer = release.indexer {
                    Text(indexer).lineLimit(1)
                }
            }
            .font(.caption)
            .foregroundStyle(.secondary)
        }
        .padding(.vertical, 2)
    }
}
