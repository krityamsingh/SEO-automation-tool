package api

const DashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>AEO + GEO + SEO Autonomous Agent | Control Center</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&family=JetBrains+Mono:wght@400;500;700&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg-gradient: linear-gradient(135deg, #0b0f19 0%, #111827 50%, #070a12 100%);
      --glass-bg: rgba(17, 24, 39, 0.7);
      --glass-border: rgba(255, 255, 255, 0.08);
      --glass-border-hover: rgba(59, 130, 246, 0.4);
      --accent-blue: #3b82f6;
      --accent-purple: #8b5cf6;
      --accent-cyan: #06b6d4;
      --accent-green: #10b981;
      --accent-amber: #f59e0b;
      --text-main: #f3f4f6;
      --text-muted: #9ca3af;
    }

    * { box-sizing: border-box; margin: 0; padding: 0; }

    body {
      font-family: 'Inter', sans-serif;
      background: var(--bg-gradient);
      color: var(--text-main);
      min-height: 100vh;
      padding: 24px;
      overflow-x: hidden;
    }

    header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      background: var(--glass-bg);
      backdrop-filter: blur(16px);
      border: 1px solid var(--glass-border);
      border-radius: 16px;
      padding: 18px 28px;
      margin-bottom: 24px;
      box-shadow: 0 8px 32px rgba(0,0,0,0.37);
    }

    .brand { display: flex; align-items: center; gap: 14px; }
    .brand-logo {
      width: 44px; height: 44px;
      background: linear-gradient(135deg, #3b82f6, #8b5cf6);
      border-radius: 12px; display: flex; align-items: center; justify-content: center; font-size: 22px;
      box-shadow: 0 0 20px rgba(139, 92, 246, 0.5);
    }
    .brand-title h1 {
      font-size: 20px; font-weight: 800;
      background: linear-gradient(90deg, #60a5fa, #c084fc);
      -webkit-background-clip: text; -webkit-text-fill-color: transparent;
    }
    .brand-title p { font-size: 12px; color: var(--text-muted); }

    .status-badges { display: flex; align-items: center; gap: 12px; }
    .badge {
      display: inline-flex; align-items: center; gap: 8px; padding: 8px 14px; border-radius: 20px; font-size: 12px; font-weight: 600;
      background: rgba(255,255,255,0.05); border: 1px solid var(--glass-border);
    }
    .badge-live { background: rgba(16, 185, 129, 0.15); border-color: rgba(16, 185, 129, 0.3); color: #34d399; }
    .badge-leader { background: rgba(139, 92, 246, 0.15); border-color: rgba(139, 92, 246, 0.3); color: #c084fc; }
    .pulse-dot { width: 8px; height: 8px; background: #34d399; border-radius: 50%; box-shadow: 0 0 8px #34d399; animation: pulse 2s infinite; }
    @keyframes pulse { 0% { opacity: 1; transform: scale(1); } 50% { opacity: 0.5; transform: scale(1.2); } 100% { opacity: 1; transform: scale(1); } }

    .dashboard-grid { display: grid; grid-template-columns: 1fr 380px; gap: 24px; }
    .card {
      background: var(--glass-bg); backdrop-filter: blur(16px); border: 1px solid var(--glass-border);
      border-radius: 16px; padding: 24px; margin-bottom: 24px; box-shadow: 0 8px 32px rgba(0,0,0,0.25);
      transition: border-color 0.3s;
    }
    .card:hover { border-color: var(--glass-border-hover); }
    .card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 18px; }
    .card-title { font-size: 16px; font-weight: 700; display: flex; align-items: center; gap: 10px; }

    .control-actions { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; margin-bottom: 24px; }
    .btn {
      display: inline-flex; align-items: center; justify-content: center; gap: 10px; padding: 14px 20px; border-radius: 12px;
      font-size: 13px; font-weight: 700; cursor: pointer; border: none; transition: all 0.2s ease; color: white;
    }
    .btn-primary { background: linear-gradient(135deg, #3b82f6, #2563eb); box-shadow: 0 4px 14px rgba(37, 99, 235, 0.4); }
    .btn-primary:hover { transform: translateY(-2px); box-shadow: 0 6px 20px rgba(37, 99, 235, 0.6); }
    .btn-research { background: linear-gradient(135deg, #8b5cf6, #7c3aed); box-shadow: 0 4px 14px rgba(124, 58, 237, 0.4); }
    .btn-research:hover { transform: translateY(-2px); box-shadow: 0 6px 20px rgba(124, 58, 237, 0.6); }
    .btn-secondary { background: rgba(255,255,255,0.08); border: 1px solid var(--glass-border); }
    .btn-secondary:hover { background: rgba(255,255,255,0.15); }

    .chat-container { height: 480px; overflow-y: auto; display: flex; flex-direction: column; gap: 14px; padding-right: 8px; }
    .chat-container::-webkit-scrollbar { width: 6px; }
    .chat-container::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.15); border-radius: 3px; }

    .chat-msg {
      display: flex; gap: 12px; background: rgba(0,0,0,0.25); border: 1px solid rgba(255,255,255,0.05);
      border-radius: 12px; padding: 14px; animation: fadeIn 0.3s ease-in-out;
    }
    @keyframes fadeIn { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); } }

    .chat-avatar { width: 36px; height: 36px; border-radius: 10px; display: flex; align-items: center; justify-content: center; font-size: 18px; flex-shrink: 0; }
    .avatar-leader { background: rgba(139, 92, 246, 0.2); border: 1px solid #8b5cf6; }
    .avatar-creator { background: rgba(59, 130, 246, 0.2); border: 1px solid #3b82f6; }
    .avatar-reviewer { background: rgba(16, 185, 129, 0.2); border: 1px solid #10b981; }

    .chat-content { flex: 1; }
    .chat-meta { display: flex; justify-content: space-between; margin-bottom: 6px; }
    .chat-sender { font-size: 13px; font-weight: 700; }
    .chat-sender.leader { color: #c084fc; }
    .chat-sender.creator { color: #60a5fa; }
    .chat-sender.reviewer { color: #34d399; }
    .chat-time { font-size: 11px; color: var(--text-muted); }
    .chat-body { font-size: 13px; line-height: 1.5; white-space: pre-wrap; font-family: 'JetBrains Mono', monospace; }

    .keys-list { display: flex; flex-direction: column; gap: 8px; }
    .key-item {
      display: flex; justify-content: space-between; align-items: center; padding: 10px 14px; border-radius: 8px;
      background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.05); font-family: 'JetBrains Mono', monospace; font-size: 12px;
    }
    .key-item.active { border-color: rgba(59, 130, 246, 0.5); background: rgba(59, 130, 246, 0.1); }

    .content-card { background: rgba(0,0,0,0.3); border: 1px solid var(--glass-border); border-radius: 12px; padding: 16px; margin-bottom: 12px; }
    .content-title { font-size: 14px; font-weight: 700; margin-bottom: 6px; }
    .content-tldr { font-size: 12px; color: var(--text-muted); margin-bottom: 12px; line-height: 1.4; }
    .content-actions { display: flex; gap: 10px; }
    .btn-sm { padding: 6px 12px; font-size: 11px; border-radius: 6px; cursor: pointer; border: none; font-weight: 700; }
    .btn-approve { background: #10b981; color: white; }
    .btn-reject { background: #ef4444; color: white; }

    .terminal-logs {
      background: #050811; border: 1px solid rgba(255,255,255,0.1); border-radius: 12px; padding: 14px;
      font-family: 'JetBrains Mono', monospace; font-size: 11px; height: 200px; overflow-y: auto; color: #34d399; line-height: 1.6;
    }
  </style>
</head>
<body>
  <header>
    <div class="brand">
      <div class="brand-logo">🧠</div>
      <div class="brand-title">
        <h1>AEO + GEO + SEO Autonomous Agent</h1>
        <p>Tripartite Multi-Agent Engine (Kimi K3 Leader + Gemini Creator + MiniMax Reviewer)</p>
      </div>
    </div>
    <div class="status-badges">
      <div class="badge badge-leader">🤖 Leader: Kimi K3 (TokenRouter)</div>
      <div class="badge badge-live">
        <div class="pulse-dot"></div> System Status: ONLINE
      </div>
    </div>
  </header>

  <div class="control-actions">
    <button class="btn btn-primary" onclick="triggerCycle()">▶ Run Autonomous Cycle Now</button>
    <button class="btn btn-research" onclick="triggerDeepResearch()">🔍 Kimi Deep Research Task</button>
    <button class="btn btn-secondary" onclick="fetchStatus()">🔄 Refresh Dashboard State</button>
  </div>

  <div class="dashboard-grid">
    <div class="left-col">
      <div class="card">
        <div class="card-header">
          <div class="card-title">💬 Live Multi-Agent Tripartite Dialogue Matrix</div>
          <span style="font-size: 12px; color: var(--text-muted)">Real-time Agent Communication</span>
        </div>
        <div class="chat-container" id="chatContainer">
          <div style="text-align:center; padding: 40px; color: var(--text-muted); font-size: 13px;">Loading multi-agent conversation feed...</div>
        </div>
      </div>

      <div class="card">
        <div class="card-header">
          <div class="card-title">📑 Generated Content Queue & Approval</div>
        </div>
        <div id="contentList">
          <div style="color: var(--text-muted); font-size: 13px;">Loading generated content queue...</div>
        </div>
      </div>
    </div>

    <div class="right-col">
      <div class="card">
        <div class="card-header">
          <div class="card-title">🔑 API Keys Pool Matrix</div>
        </div>
        <div class="keys-list" id="keysList">
          <div style="color: var(--text-muted); font-size: 12px;">Loading API key pool...</div>
        </div>
      </div>

      <div class="card">
        <div class="card-header">
          <div class="card-title">💻 Live Agent Activity Logs</div>
        </div>
        <div class="terminal-logs" id="logsTerminal">Initializing system log connection...</div>
      </div>
    </div>
  </div>

  <script>
    async function fetchChat() {
      try {
        const res = await fetch('/api/agent-chat');
        const messages = await res.json();
        const container = document.getElementById('chatContainer');
        if (!messages || messages.length === 0) {
          container.innerHTML = '<div style="text-align:center; padding: 40px; color: var(--text-muted); font-size: 13px;">No multi-agent messages yet. Trigger an autonomous cycle to watch Kimi K3, Gemini, and MiniMax collaborate!</div>';
          return;
        }
        let html = '';
        for (let i = 0; i < messages.length; i++) {
          const m = messages[i];
          html += '<div class="chat-msg">';
          html += '<div class="chat-avatar avatar-' + m.role + '">' + (m.avatar || '🤖') + '</div>';
          html += '<div class="chat-content">';
          html += '<div class="chat-meta">';
          html += '<span class="chat-sender ' + m.role + '">' + m.sender + '</span>';
          html += '<span class="chat-time">' + new Date(m.timestamp).toLocaleTimeString() + '</span>';
          html += '</div>';
          html += '<div class="chat-body">' + escapeHTML(m.message) + '</div>';
          html += '</div>';
          html += '</div>';
        }
        container.innerHTML = html;
        container.scrollTop = container.scrollHeight;
      } catch (err) {
        console.error('Failed to fetch agent chat:', err);
      }
    }

    async function fetchKeys() {
      try {
        const res = await fetch('/api-keys');
        const data = await res.json();
        const list = document.getElementById('keysList');
        if (!data.keys) return;
        let html = '';
        for (let i = 0; i < data.keys.length; i++) {
          const k = data.keys[i];
          const activeClass = k.is_active ? 'active' : '';
          html += '<div class="key-item ' + activeClass + '">';
          html += '<span>#' + k.index + ' [' + k.provider.toUpperCase() + '] ' + k.key_mask + '</span>';
          html += '<button class="btn-sm btn-approve" onclick="selectKey(' + k.index + ')">Select</button>';
          html += '</div>';
        }
        list.innerHTML = html;
      } catch (err) {
        console.error('Failed to fetch keys:', err);
      }
    }

    async function fetchContent() {
      try {
        const res = await fetch('/content');
        const items = await res.json();
        const list = document.getElementById('contentList');
        if (!items || items.length === 0) {
          list.innerHTML = '<div style="color: var(--text-muted); font-size: 13px;">No content created yet. Click "Run Autonomous Cycle Now" above.</div>';
          return;
        }
        let html = '';
        for (let i = 0; i < items.length; i++) {
          const c = items[i];
          html += '<div class="content-card">';
          html += '<div class="content-title">' + escapeHTML(c.title) + '</div>';
          html += '<div class="content-tldr">' + escapeHTML(c.tldr || c.meta_description) + '</div>';
          html += '<div class="content-actions">';
          if (c.status === 'pending_review' || c.status === 'draft') {
            html += '<button class="btn-sm btn-approve" onclick="approveContent(' + c.id + ')">Approve & Publish</button>';
            html += '<button class="btn-sm btn-reject" onclick="rejectContent(' + c.id + ')">Reject</button>';
          } else {
            html += '<span style="font-size:11px; color:#10b981; font-weight:bold;">Status: ' + c.status.toUpperCase() + '</span>';
          }
          html += '</div>';
          html += '</div>';
        }
        list.innerHTML = html;
      } catch (err) {
        console.error('Failed to fetch content:', err);
      }
    }

    async function fetchLogs() {
      try {
        const res = await fetch('/logs');
        const logs = await res.json();
        const term = document.getElementById('logsTerminal');
        if (!logs) return;
        let html = '';
        for (let i = 0; i < logs.length; i++) {
          const l = logs[i];
          html += '[' + new Date(l.created_at).toLocaleTimeString() + '] [' + l.action.toUpperCase() + '] ' + escapeHTML(l.message) + '<br>';
        }
        term.innerHTML = html;
        term.scrollTop = term.scrollHeight;
      } catch (err) {
        console.error('Failed to fetch logs:', err);
      }
    }

    async function triggerCycle() {
      alert('🚀 Triggering Autonomous Agent Cycle...');
      await fetch('/trigger', { method: 'POST' });
      setTimeout(fetchStatus, 2000);
    }

    async function triggerDeepResearch() {
      const topic = prompt('Enter research topic for Kimi K3 Leader Agent:', 'Future of Generative Engine Optimization in 2026');
      if (!topic) return;
      alert('🔍 Kimi K3 Leader Agent is starting Deep Research on: ' + topic);
      await fetch('/trigger', { method: 'POST' });
      setTimeout(fetchStatus, 2000);
    }

    async function selectKey(idx) {
      await fetch('/api-keys/select', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ index: idx })
      });
      fetchKeys();
    }

    async function approveContent(id) {
      await fetch('/content/' + id + '/approve', { method: 'POST' });
      fetchContent();
    }

    async function rejectContent(id) {
      await fetch('/content/' + id + '/reject', { method: 'POST' });
      fetchContent();
    }

    function fetchStatus() {
      fetchChat();
      fetchKeys();
      fetchContent();
      fetchLogs();
    }

    function escapeHTML(str) {
      if (!str) return '';
      return str.replace(/[&<>'"]/g, function(tag) {
        return { '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag;
      });
    }

    fetchStatus();
    setInterval(fetchStatus, 3000);
  </script>
</body>
</html>
`
