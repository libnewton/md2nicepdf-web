const el = (id) => document.getElementById(id);

let easyMDE;

async function build() {
  try {
    setStatus('Building…');
    let token = localStorage.getItem('auth_token');
    if (!token) {
      token = prompt("Please enter the internal auth token:");
      if (token) {
        localStorage.setItem('auth_token', token);
      } else {
        setStatus('Token missing');
        return;
      }
    }

    const raw = easyMDE ? easyMDE.value() : el('markdown').value;
    let markdown = typeof raw === 'string' ? raw : String(raw || '');

    const body = { md: markdown };
    if (el('noPageNumbers').checked) {
      body.no_page_numbers = true;
    }

    const res = await fetch('/pdf', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'internal-auth': token
      },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      if (res.status === 401) {
        localStorage.removeItem('auth_token'); // Clear invalid token
      }
      // Parse error response safely — never show raw internal details
      let userMessage = 'Build failed. Please try again.';
      try {
        const errData = await res.json();
        if (errData.detail) {
          userMessage = errData.detail;
        }
      } catch (_) {
        // Response wasn't JSON (e.g. proxy error) — use generic message
      }
      showErrorModal(userMessage);
      throw new Error(userMessage);
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    el('pdfFrame').src = url;

    const downloadBtn = el('downloadBtn');
    downloadBtn.href = url;
    downloadBtn.download = "document" + '.pdf';
    downloadBtn.style.display = 'inline-block';

    setStatus('Built ✓');
  } catch (e) {
    console.error(e);
    setStatus('Build failed');
  }
}

function setStatus(msg) {
  el('status').textContent = msg;
}



document.addEventListener('DOMContentLoaded', () => {
  // Initialize EasyMDE markdown editor
  const textarea = el('markdown');
  if (window.EasyMDE && window.CodeMirror && textarea) {
    try {
      // In case a stale autosave value exists from prior versions
      try { localStorage.removeItem('md2pdf-editor'); } catch (_) { }
      easyMDE = new EasyMDE({
        element: textarea,
        autofocus: true,
        spellChecker: false,
        status: false,
        toolbar: false,
        codemirror: {
          mode: {
            name: 'gfm',
            fencedCodeBlockHighlighting: true,
            highlightFormatting: false,
            tokenTypeOverrides: { code: 'atom' } // make unfenced/unknown code less gray
          },
          lineWrapping: true,
          lineNumbers: false,
          extraKeys: {
            "Ctrl-Enter": function (cm) {
              build();
            }
          },
        },
        shortcuts: {
          toggleSideBySide: null,
          toggleFullScreen: null,
          togglePreview: null,
        },
        // Autosave disabled to avoid edge cases with stored non-string values
        sideBySideFullscreen: false,
        renderingConfig: { singleLineBreaks: false, codeSyntaxHighlighting: true },
        autoDownloadFontAwesome: false,
        forceSync: true,
      });

      // Load saved content
      const savedContent = localStorage.getItem('md2pdf-content');
      if (savedContent) {
        easyMDE.value(savedContent);
      }

      // Save on change
      easyMDE.codemirror.on("change", () => {
        localStorage.setItem('md2pdf-content', easyMDE.value());
      });

    } catch (err) {
      console.error('EasyMDE init failed, falling back to textarea:', err);
      easyMDE = null;
    }
  }
  el('buildBtn').addEventListener('click', build);
  document.addEventListener('keydown', (e) => {
    if (e.ctrlKey && e.key === 'Enter') {
      e.preventDefault();
      build();
    }
    if (e.key === 'Escape') {
      closeErrorModal();
    }
  });
  el('errorModalClose').addEventListener('click', closeErrorModal);
  el('errorModal').addEventListener('click', (ev) => {
    if (ev.target.id === 'errorModal') closeErrorModal();
  });
});

function showErrorModal(text) {
  el('errorLogs').textContent = text || '';
  el('errorModal').classList.add('open');
}

function closeErrorModal() {
  el('errorModal').classList.remove('open');
}
document.onreadystatechange = function () {
  if (document.readyState === 'complete') {
    const setInitial = (val) => {
      if (localStorage.getItem('md2pdf-content')) return; // Don't overwrite saved content
      if (easyMDE) {
        if ((easyMDE.value() || '').trim() === '') easyMDE.value(val);
      } else {
        let mdContainer = document.getElementById('markdown');
        if (mdContainer && mdContainer.value === '') mdContainer.value = val;
      }
    };
    setInitial(`# Welcome to Markdown 2 PDF`);
  }
}
