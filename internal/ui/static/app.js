// Sidekick Web UI

const $ = (sel) => document.querySelector(sel);
const app = $('#app');
const apiKeyInput = $('#api-key');

// Restore API key from session storage.
apiKeyInput.value = sessionStorage.getItem('sidekick_api_key') || '';
apiKeyInput.addEventListener('input', () => {
  sessionStorage.setItem('sidekick_api_key', apiKeyInput.value);
});

function getKey() {
  return apiKeyInput.value.trim();
}

async function api(method, path, body) {
  const key = getKey();
  if (!key) throw new Error('API key is required');

  const opts = {
    method,
    headers: { 'X-Sidekick-Key': key },
  };
  if (body) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }

  const resp = await fetch(path, opts);
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({}));
    throw new Error(err.error || `HTTP ${resp.status}`);
  }
  return resp.json();
}

// --- Router ---

function route() {
  const hash = location.hash || '#/';

  if (hash === '#/submit') {
    renderSubmit();
  } else if (hash.startsWith('#/tasks/')) {
    const id = hash.slice('#/tasks/'.length);
    renderTaskDetail(id);
  } else {
    renderTaskList();
  }
}

window.addEventListener('hashchange', route);
window.addEventListener('load', route);

// --- Task List ---

async function renderTaskList() {
  if (!getKey()) {
    app.innerHTML = '<p class="muted">Enter your API key above to get started.</p>';
    return;
  }

  app.innerHTML = '<p class="muted">Loading...</p>';

  try {
    const tasks = await api('GET', '/tasks?limit=50');
    if (!tasks.length) {
      app.innerHTML = '<p class="muted">No tasks yet. <a href="#/submit">Submit one</a>.</p>';
      return;
    }

    app.innerHTML = `
      <table>
        <thead>
          <tr><th>ID</th><th>Status</th><th>Workflow</th><th>Created</th></tr>
        </thead>
        <tbody>
          ${tasks.map(t => `
            <tr>
              <td><a href="#/tasks/${t.id}">${t.id}</a></td>
              <td><span class="status status-${t.status}">${t.status}</span></td>
              <td>${esc(t.workflow)}</td>
              <td>${fmtTime(t.created_at)}</td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    `;
  } catch (e) {
    app.innerHTML = `<p class="error-msg">${esc(e.message)}</p>`;
  }
}

// --- Task Detail ---

let activeStream = null;

async function renderTaskDetail(id) {
  // Abort any previous stream.
  if (activeStream) { activeStream.abort(); activeStream = null; }

  if (!getKey()) {
    app.innerHTML = '<p class="muted">Enter your API key above.</p>';
    return;
  }

  app.innerHTML = '<p class="muted">Loading...</p>';

  try {
    const t = await api('GET', `/tasks/${id}`);

    app.innerHTML = `
      <div class="task-detail">
        <h2>${esc(t.id)}</h2>
        <dl class="task-meta">
          <dt>Status</dt><dd><span class="status status-${t.status}">${t.status}</span></dd>
          <dt>Workflow</dt><dd>${esc(t.workflow)}</dd>
          <dt>Created</dt><dd>${fmtTime(t.created_at)}</dd>
          ${t.started_at ? `<dt>Started</dt><dd>${fmtTime(t.started_at)}</dd>` : ''}
          ${t.completed_at ? `<dt>Finished</dt><dd>${fmtTime(t.completed_at)}</dd>` : ''}
          ${t.error ? `<dt>Error</dt><dd class="error-msg">${esc(t.error)}</dd>` : ''}
          ${t.total_tokens_used ? `<dt>Tokens</dt><dd>${t.total_tokens_used}</dd>` : ''}
        </dl>
        ${t.steps && t.steps.length ? `
          <table class="steps-table">
            <thead><tr><th>Step</th><th>Status</th><th>Duration</th><th>Tokens</th></tr></thead>
            <tbody>
              ${t.steps.map(s => `
                <tr>
                  <td>${esc(s.name)}</td>
                  <td><span class="status status-${s.status}">${s.status}</span></td>
                  <td>${s.duration_ms ? (s.duration_ms / 1000).toFixed(1) + 's' : '-'}</td>
                  <td>${s.tokens_used || '-'}</td>
                </tr>
              `).join('')}
            </tbody>
          </table>
        ` : ''}
        <h3>Events</h3>
        <div class="log-viewer" id="log-viewer"></div>
      </div>
    `;

    streamEvents(id);
  } catch (e) {
    app.innerHTML = `<p class="error-msg">${esc(e.message)}</p>`;
  }
}

async function streamEvents(taskID) {
  const key = getKey();
  if (!key) return;

  const viewer = $('#log-viewer');
  if (!viewer) return;

  const controller = new AbortController();
  activeStream = controller;

  try {
    const resp = await fetch(`/tasks/${taskID}/stream`, {
      headers: { 'X-Sidekick-Key': key, 'Accept': 'text/event-stream' },
      signal: controller.signal,
    });

    if (!resp.ok) {
      viewer.innerHTML = `<div class="log-line error-msg">Failed to connect: HTTP ${resp.status}</div>`;
      return;
    }

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let eventType = '';
    let dataLines = [];

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop(); // Keep incomplete line in buffer.

      for (const line of lines) {
        if (line.startsWith('event: ')) {
          eventType = line.slice(7);
        } else if (line.startsWith('data: ')) {
          dataLines.push(line.slice(6));
        } else if (line === '' && eventType) {
          const data = dataLines.join('\n');
          appendEvent(viewer, eventType, data);
          if (eventType === 'task.completed') {
            activeStream = null;
            return;
          }
          eventType = '';
          dataLines = [];
        }
      }
    }
  } catch (e) {
    if (e.name !== 'AbortError') {
      viewer.innerHTML += `<div class="log-line error-msg">Stream error: ${esc(e.message)}</div>`;
    }
  }

  activeStream = null;
}

function appendEvent(viewer, type, dataStr) {
  let d;
  try { d = JSON.parse(dataStr); } catch { return; }

  const div = document.createElement('div');
  div.className = 'log-line';

  switch (type) {
    case 'task.started':
      div.className += ' log-step-start';
      div.textContent = 'Task started';
      break;
    case 'task.completed':
      div.className += ` log-task-done ${d.status}`;
      div.textContent = `Task ${d.status}` + (d.total_tokens_used ? ` (${d.total_tokens_used} tokens)` : '');
      break;
    case 'step.started':
      div.className += ' log-step-start';
      div.textContent = `>> ${d.step}`;
      break;
    case 'step.completed':
      div.className += ` log-step-done ${d.status}`;
      div.textContent = `   ${d.step}: ${d.status} (${(d.duration_ms / 1000).toFixed(1)}s)` +
        (d.tokens_used ? ` ${d.tokens_used} tokens` : '');
      break;
    case 'step.skipped':
      div.className += ' log-thinking';
      div.textContent = `   ${d.step}: skipped (${d.reason})`;
      break;
    case 'step.output':
      div.className += d.stream === 'stderr' ? ' log-stderr' : ' log-output';
      div.textContent = `   ${d.line}`;
      break;
    case 'agent.thinking':
      div.className += ' log-thinking';
      div.textContent = `   [thinking] ${d.text}`;
      break;
    case 'agent.action':
      div.className += ' log-action';
      div.textContent = `   [${d.tool}] ${d.detail}`;
      break;
    case 'agent.action_result':
      div.className += ' log-stderr';
      const lines = (d.output || '').split('\n');
      div.textContent = lines.slice(0, 5).map(l => '   ' + l).join('\n') +
        (lines.length > 5 ? `\n   ... (${lines.length - 5} more lines)` : '');
      break;
    case 'agent.output':
      div.className += ' log-agent-output';
      div.textContent = `   ${d.text}`;
      break;
    default:
      div.className += ' log-thinking';
      div.textContent = `   [${type}]`;
  }

  viewer.appendChild(div);
  viewer.scrollTop = viewer.scrollHeight;
}

// --- Submit Form ---

function renderSubmit() {
  if (!getKey()) {
    app.innerHTML = '<p class="muted">Enter your API key above.</p>';
    return;
  }

  app.innerHTML = `
    <h2>Submit Task</h2>
    <form id="submit-form">
      <div class="form-group">
        <label for="workflow">Workflow</label>
        <input type="text" id="workflow" placeholder="fix-issue" required>
      </div>
      <div class="form-group">
        <label>Variables</label>
        <div id="vars-container"></div>
        <button type="button" class="secondary" onclick="addVarRow()">+ Add variable</button>
      </div>
      <div class="form-group">
        <label for="webhook">Webhook URL (optional)</label>
        <input type="url" id="webhook" placeholder="https://example.com/hooks/sidekick">
      </div>
      <div class="form-actions">
        <button type="submit" class="primary">Submit</button>
        <label style="font-size:0.85rem"><input type="checkbox" id="follow-check"> Stream events after submit</label>
      </div>
      <div id="submit-error"></div>
    </form>
  `;

  addVarRow();

  $('#submit-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const btn = e.target.querySelector('button[type="submit"]');
    btn.disabled = true;
    $('#submit-error').textContent = '';

    const workflow = $('#workflow').value.trim();
    const webhook = $('#webhook').value.trim();
    const follow = $('#follow-check').checked;

    const variables = {};
    document.querySelectorAll('.var-row').forEach(row => {
      const key = row.querySelector('.var-key').value.trim();
      const val = row.querySelector('.var-val').value.trim();
      if (key) variables[key] = val;
    });

    try {
      const body = { workflow, variables };
      if (webhook) body.webhook_url = webhook;
      const task = await api('POST', '/tasks', body);

      if (follow) {
        location.hash = `#/tasks/${task.id}`;
      } else {
        location.hash = '#/';
      }
    } catch (err) {
      $('#submit-error').innerHTML = `<p class="error-msg">${esc(err.message)}</p>`;
      btn.disabled = false;
    }
  });
}

// Expose globally for inline onclick.
window.addVarRow = function() {
  const container = $('#vars-container');
  const row = document.createElement('div');
  row.className = 'var-row';
  row.innerHTML = `
    <input type="text" class="var-key" placeholder="KEY">
    <input type="text" class="var-val" placeholder="VALUE">
    <button type="button" onclick="this.parentElement.remove()">x</button>
  `;
  container.appendChild(row);
};

window.removeVarRow = function(btn) {
  btn.parentElement.remove();
};

// --- Helpers ---

function esc(s) {
  const d = document.createElement('div');
  d.textContent = s || '';
  return d.innerHTML;
}

function fmtTime(iso) {
  if (!iso) return '-';
  const d = new Date(iso);
  return d.toLocaleString();
}
