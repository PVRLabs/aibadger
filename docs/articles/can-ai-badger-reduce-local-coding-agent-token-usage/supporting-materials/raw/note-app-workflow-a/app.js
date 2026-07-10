const NOTES_KEY = 'notes_app_notes';
const UI_KEY = 'notes_app_ui';
const SETTINGS_KEY = 'notes_app_settings';

const state = {
  notes: [],
  selectedId: null,
  editingId: null,
  deletingId: null,
  ui: {
    search: '',
    tag: 'all',
    sort: 'updated-newest',
  },
  settings: {
    theme: 'system',
    sort: 'updated-newest',
  },
};

const els = {
  noteModal: document.getElementById('noteModal'),
  deleteModal: document.getElementById('deleteModal'),
  notesContainer: document.getElementById('notesContainer'),
  searchInput: document.getElementById('searchInput'),
  tagFilter: document.getElementById('tagFilter'),
  addNoteBtn: document.getElementById('addNoteBtn'),
  closeModalBtn: document.getElementById('closeModalBtn'),
  cancelBtn: document.getElementById('cancelBtn'),
  saveBtn: document.getElementById('saveBtn'),
  noteTitle: document.getElementById('noteTitle'),
  noteBody: document.getElementById('noteBody'),
  noteTags: document.getElementById('noteTags'),
  deleteCancelBtn: document.getElementById('deleteCancelBtn'),
  deleteConfirmBtn: document.getElementById('deleteConfirmBtn'),
  settingsBtn: document.getElementById('settingsBtn'),
  settingsModal: document.getElementById('settingsModal'),
  closeSettingsBtn: document.getElementById('closeSettingsBtn'),
  settingsTheme: document.getElementById('settingsTheme'),
  settingsSort: document.getElementById('settingsSort'),
};

function uid() {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
}

function now() {
  return Date.now();
}

function readJSON(key, fallback) {
  try {
    const raw = localStorage.getItem(key);
    return raw ? JSON.parse(raw) : fallback;
  } catch {
    return fallback;
  }
}

function saveJSON(key, value) {
  localStorage.setItem(key, JSON.stringify(value));
}

function parseTags(value) {
  return value
    .split(',')
    .map(tag => tag.trim())
    .filter(Boolean)
    .filter((tag, index, array) => array.indexOf(tag) === index);
}

function normalizeNote(note) {
  const timestamp = Number(note.updatedAt || note.createdAt || now());
  const createdAt = Number(note.createdAt || timestamp);
  return {
    id: String(note.id || uid()),
    title: String(note.title || ''),
    body: String(note.body || ''),
    tags: Array.isArray(note.tags) ? note.tags.map(String).filter(Boolean) : [],
    createdAt,
    updatedAt: Number(note.updatedAt || createdAt),
  };
}

function loadNotes() {
  const notes = readJSON(NOTES_KEY, []);
  return Array.isArray(notes) ? notes.map(normalizeNote) : [];
}

function loadUI() {
  const ui = readJSON(UI_KEY, {});
  return {
    search: String(ui.search || ''),
    tag: String(ui.tag || 'all'),
    sort: ['updated-newest', 'updated-oldest', 'title-az'].includes(ui.sort) ? ui.sort : 'updated-newest',
    selectedId: ui.selectedId ? String(ui.selectedId) : null,
  };
}

function saveNotes() {
  saveJSON(NOTES_KEY, state.notes);
}

function saveUI() {
  saveJSON(UI_KEY, {
    search: state.ui.search,
    tag: state.ui.tag,
    sort: state.ui.sort,
    selectedId: state.selectedId,
  });
}

function loadSettings() {
  const saved = readJSON(SETTINGS_KEY, {});
  return {
    theme: ['light', 'dark', 'system'].includes(saved.theme) ? saved.theme : 'system',
    sort: ['updated-newest', 'updated-oldest', 'title-az'].includes(saved.sort) ? saved.sort : 'updated-newest',
  };
}

function saveSettings() {
  saveJSON(SETTINGS_KEY, {
    theme: state.settings.theme,
    sort: state.settings.sort,
  });
}

let systemThemeMediaQuery;

function applyTheme(theme) {
  theme = theme || state.settings.theme;

  if (theme === 'dark') {
    document.documentElement.setAttribute('data-theme', 'dark');
  } else if (theme === 'light') {
    document.documentElement.setAttribute('data-theme', 'light');
  } else {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    document.documentElement.setAttribute('data-theme', prefersDark ? 'dark' : 'light');
  }

  if (systemThemeMediaQuery) {
    systemThemeMediaQuery.removeEventListener('change', handleSystemThemeChange);
  }

  if (theme === 'system') {
    systemThemeMediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    systemThemeMediaQuery.addEventListener('change', handleSystemThemeChange);
  }
}

function handleSystemThemeChange(event) {
  if (state.settings.theme === 'system') {
    document.documentElement.setAttribute('data-theme', event.matches ? 'dark' : 'light');
  }
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function getFilteredNotes() {
  const query = state.ui.search.trim().toLowerCase();
  const tag = state.ui.tag;

  return state.notes
    .filter(note => {
      const matchesQuery = !query || note.title.toLowerCase().includes(query) || note.body.toLowerCase().includes(query);
      const matchesTag = tag === 'all' || note.tags.includes(tag);
      return matchesQuery && matchesTag;
    })
    .sort((a, b) => {
      if (state.ui.sort === 'updated-oldest') return a.updatedAt - b.updatedAt;
      if (state.ui.sort === 'title-az') return a.title.localeCompare(b.title, undefined, { sensitivity: 'base' });
      return b.updatedAt - a.updatedAt;
    });
}

function getTagOptions() {
  return [...new Set(state.notes.flatMap(note => note.tags))].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }));
}

function selectNote(id) {
  state.selectedId = id;
  saveUI();
  render();
}

function renderTagFilter() {
  const current = state.ui.tag;
  const tags = getTagOptions();
  els.tagFilter.innerHTML = [
    '<option value="all">All tags</option>',
    ...tags.map(tag => `<option value="${escapeHtml(tag)}">${escapeHtml(tag)}</option>`),
  ].join('');
  const nextTag = tags.includes(current) || current === 'all' ? current : 'all';
  els.tagFilter.value = nextTag;
  if (state.ui.tag !== nextTag) {
    state.ui.tag = nextTag;
    saveUI();
  }
}

function renderNotes() {
  const filtered = getFilteredNotes();

  if (filtered.length === 0) {
    els.notesContainer.innerHTML = `
      <div class="empty-state">
        <p>${state.notes.length === 0 ? 'No notes yet. Create one.' : 'No notes match your current search or filter.'}</p>
      </div>`;
    return;
  }

  els.notesContainer.innerHTML = filtered.map(note => {
    const selectedClass = note.id === state.selectedId ? ' selected' : '';
    const tags = note.tags.slice(0, 3).map(tag => `<span class="tag">${escapeHtml(tag)}</span>`).join('');
    const moreTags = note.tags.length > 3 ? `<span class="tag muted">+${note.tags.length - 3}</span>` : '';
    return `
      <article class="note-card${selectedClass}" data-id="${note.id}" tabindex="0" role="button" aria-pressed="${note.id === state.selectedId}">
        <div class="note-card-title">${escapeHtml(note.title || 'Untitled')}</div>
        <div class="note-card-body">${escapeHtml(note.body || '')}</div>
        <div class="note-card-tags">${tags}${moreTags}</div>
        <div class="note-card-footer">
          <button class="note-card-btn" data-edit="${note.id}">Edit</button>
          <button class="note-card-btn danger" data-delete="${note.id}">Delete</button>
        </div>
      </article>
    `;
  }).join('');

  if (state.selectedId && !filtered.some(note => note.id === state.selectedId)) {
    state.selectedId = filtered[0].id;
    saveUI();
  }
}

function render() {
  renderTagFilter();
  renderNotes();
}

function openNoteModal(note) {
  state.editingId = note ? note.id : null;
  document.getElementById('modalTitle').textContent = note ? 'Edit Note' : 'New Note';
  els.noteTitle.value = note ? note.title : '';
  els.noteBody.value = note ? note.body : '';
  els.noteTags.value = note ? note.tags.join(', ') : '';
  els.noteModal.classList.remove('hidden');
  els.noteTitle.focus();
}

function closeNoteModal() {
  els.noteModal.classList.add('hidden');
  state.editingId = null;
  els.noteTitle.value = '';
  els.noteBody.value = '';
  els.noteTags.value = '';
}

function saveNote() {
  const title = els.noteTitle.value.trim();
  const body = els.noteBody.value.trim();
  const tags = parseTags(els.noteTags.value);

  if (!title && !body && tags.length === 0) {
    closeNoteModal();
    return;
  }

  if (state.editingId) {
    const note = state.notes.find(item => item.id === state.editingId);
    if (note) {
      note.title = title;
      note.body = body;
      note.tags = tags;
      note.updatedAt = now();
      state.selectedId = note.id;
    }
  } else {
    const timestamp = now();
    const note = normalizeNote({
      id: uid(),
      title,
      body,
      tags,
      createdAt: timestamp,
      updatedAt: timestamp,
    });
    state.notes.push(note);
    state.selectedId = note.id;
  }

  saveNotes();
  saveUI();
  closeNoteModal();
  render();
}

function openDeleteModal(id) {
  state.deletingId = id;
  els.deleteModal.classList.remove('hidden');
}

function closeDeleteModal() {
  els.deleteModal.classList.add('hidden');
  state.deletingId = null;
}

function confirmDelete() {
  if (state.deletingId) {
    const wasSelected = state.deletingId === state.selectedId;
    state.notes = state.notes.filter(note => note.id !== state.deletingId);
    if (wasSelected) {
      const nextVisible = getFilteredNotes()[0];
      state.selectedId = nextVisible ? nextVisible.id : null;
    }
    saveNotes();
    saveUI();
    render();
  }
  closeDeleteModal();
}

function handleCardClick(event) {
  const editBtn = event.target.closest('[data-edit]');
  const deleteBtn = event.target.closest('[data-delete]');
  const card = event.target.closest('[data-id]');

  if (editBtn) {
    const note = state.notes.find(item => item.id === editBtn.dataset.edit);
    if (note) openNoteModal(note);
    return;
  }

  if (deleteBtn) {
    openDeleteModal(deleteBtn.dataset.delete);
    return;
  }

  if (card) {
    selectNote(card.dataset.id);
  }
}

function syncStateFromInputs() {
  state.ui.search = els.searchInput.value;
  state.ui.tag = els.tagFilter.value;
}

els.notesContainer.addEventListener('click', handleCardClick);
els.notesContainer.addEventListener('keydown', event => {
  if (event.key === 'Enter' || event.key === ' ') {
    const card = event.target.closest('[data-id]');
    if (card) {
      event.preventDefault();
      selectNote(card.dataset.id);
    }
  }
});

els.searchInput.addEventListener('input', () => {
  syncStateFromInputs();
  saveUI();
  render();
});

els.tagFilter.addEventListener('change', () => {
  syncStateFromInputs();
  saveUI();
  render();
});

function openSettingsModal() {
  els.settingsTheme.value = state.settings.theme;
  els.settingsSort.value = state.settings.sort;
  els.settingsModal.classList.remove('hidden');
}

function closeSettingsModal() {
  els.settingsModal.classList.add('hidden');
}

function applySettings() {
  const theme = els.settingsTheme.value;
  const sort = els.settingsSort.value;
  state.settings.theme = theme;
  state.settings.sort = sort;
  state.ui.sort = sort;
  applyTheme(theme);
  saveSettings();
  saveUI();
  render();
}

els.settingsBtn.addEventListener('click', openSettingsModal);
els.closeSettingsBtn.addEventListener('click', closeSettingsModal);
els.settingsModal.addEventListener('click', event => {
  if (event.target === els.settingsModal) closeSettingsModal();
});
els.settingsTheme.addEventListener('change', applySettings);
els.settingsSort.addEventListener('change', applySettings);

els.addNoteBtn.addEventListener('click', () => openNoteModal(null));
els.closeModalBtn.addEventListener('click', closeNoteModal);
els.cancelBtn.addEventListener('click', closeNoteModal);
els.noteModal.addEventListener('click', event => {
  if (event.target === els.noteModal) closeNoteModal();
});
els.saveBtn.addEventListener('click', saveNote);

els.noteTitle.addEventListener('keydown', event => {
  if (event.key === 'Enter') {
    event.preventDefault();
    els.noteBody.focus();
  }
});

els.noteBody.addEventListener('keydown', event => {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault();
    saveNote();
  }
});

els.deleteCancelBtn.addEventListener('click', closeDeleteModal);
els.deleteConfirmBtn.addEventListener('click', confirmDelete);
els.deleteModal.addEventListener('click', event => {
  if (event.target === els.deleteModal) closeDeleteModal();
});

document.addEventListener('keydown', event => {
  if (event.key === 'Escape') {
    if (!els.settingsModal.classList.contains('hidden')) closeSettingsModal();
    else if (!els.deleteModal.classList.contains('hidden')) closeDeleteModal();
    else if (!els.noteModal.classList.contains('hidden')) closeNoteModal();
  }
});

state.notes = loadNotes();
state.ui = loadUI();
state.settings = loadSettings();
state.ui.sort = state.settings.sort;
state.selectedId = state.notes.some(note => note.id === state.ui.selectedId) ? state.ui.selectedId : (state.notes[0] ? state.notes[0].id : null);
els.searchInput.value = state.ui.search;
applyTheme(state.settings.theme);
render();
