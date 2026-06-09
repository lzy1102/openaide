// OpenAIDE Desktop — Wails Frontend
// Calls Go backend via window.go.main.App methods

const chat = {
  messages: document.getElementById('messages'),
  input: document.getElementById('input'),
  toolStatus: document.getElementById('tool-status'),
  diffViewer: document.getElementById('diff-viewer'),
  sessions: document.getElementById('sessions'),
  files: document.getElementById('files'),
};

async function sendMessage() {
  const text = chat.input.value.trim();
  if (!text) return;

  addMessage('user', text);
  chat.input.value = '';

  try {
    const response = await window.go.main.App.SendMessage(text);
    addMessage('assistant', response);
  } catch (err) {
    addMessage('assistant', `Error: ${err}`);
  }

  loadSessions();
}

function addMessage(role, content) {
  const div = document.createElement('div');
  div.className = `message ${role}`;

  if (role === 'user') {
    div.innerHTML = `<span class="bubble">${escapeHtml(content)}</span>`;
  } else {
    // Simple markdown rendering
    let html = content
      .replace(/```(\w*)\n([\s\S]*?)```/g, '<pre><code>$2</code></pre>')
      .replace(/`([^`]+)`/g, '<code>$1</code>')
      .replace(/^### (.+)$/gm, '<h3>$1</h3>')
      .replace(/^## (.+)$/gm, '<h2>$1</h2>')
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(/\n/g, '<br>');

    div.innerHTML = `<div class="bubble">${html}</div>`;
  }

  chat.messages.appendChild(div);
  chat.messages.scrollTop = chat.messages.scrollHeight;
}

async function loadSessions() {
  try {
    const sessions = await window.go.main.App.GetSessions();
    chat.sessions.innerHTML = sessions
      .map(s => `<div class="session-item" onclick="switchSession('${s.id}')">${s.id.slice(0, 8)} — ${s.updated_at}</div>`)
      .join('');
  } catch (e) {
    // Wails bridge not yet available (dev mode without wails)
  }
}

async function loadFileTree() {
  try {
    const tree = await window.go.main.App.GetFileTree('.');
    chat.files.innerHTML = renderFileTree(tree);
  } catch (e) {
    chat.files.innerHTML = '<div class="file-item">(file tree unavailable)</div>';
  }
}

function renderFileTree(nodes) {
  return nodes.map(n => {
    const cls = n.is_dir ? 'dir' : 'file';
    const children = n.children ? renderFileTree(n.children) : '';
    return `<div class="file-item ${cls}" onclick="${n.is_dir ? '' : `readFile('${n.path}')`}">${n.name}</div>${children}`;
  }).join('');
}

async function readFile(path) {
  try {
    const content = await window.go.main.App.ReadFileContent(path);
    chat.diffViewer.innerHTML = `<h4>${path}</h4><pre>${escapeHtml(content.slice(0, 5000))}</pre>`;
  } catch (e) {
    chat.diffViewer.innerHTML = `<pre>Error reading file</pre>`;
  }
}

function newSession() {
  chat.messages.innerHTML = '';
  chat.input.value = '';
  chat.input.focus();
}

function switchSession(id) {
  // TODO: load session messages
  console.log('Switch to session:', id);
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

// Init
chat.input.addEventListener('keydown', e => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendMessage();
  }
});

loadSessions();
loadFileTree();
