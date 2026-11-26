// NGINX++ browser bundle: CodeMirror 6 editor + dagre graph layout.
// Built by scripts/build-assets.sh (esbuild) into static/vendor/bundle.js,
// exposed as window.NP so the plain-JS UI can use it without a framework.
import { EditorState, Compartment } from '@codemirror/state';
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection, hoverTooltip } from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { StreamLanguage, syntaxHighlighting, defaultHighlightStyle, bracketMatching, indentOnInput } from '@codemirror/language';
import { searchKeymap, highlightSelectionMatches } from '@codemirror/search';
import { autocompletion, completionKeymap, closeBrackets } from '@codemirror/autocomplete';
import { linter, lintGutter, forceLinting } from '@codemirror/lint';
import { oneDark } from '@codemirror/theme-one-dark';
import { nginx } from '@codemirror/legacy-modes/mode/nginx';
import dagre from '@dagrejs/dagre';

function wordAt(state, pos) {
  const line = state.doc.lineAt(pos);
  const text = line.text;
  let s = pos - line.from, e = s;
  while (s > 0 && /[a-z_0-9]/.test(text[s - 1])) s--;
  while (e < text.length && /[a-z_0-9]/.test(text[e])) e++;
  return s === e ? null : { from: line.from + s, to: line.from + e, word: text.slice(s, e) };
}

/**
 * createEditor(parent, opts) → { get, set, gotoLine, insert, cursor, relint, setTheme }
 * opts.docs: { directive: [syntax, description, docPath] }
 * opts.lint: async (text) => [{ line, severity, message, doc }]
 * opts.onChange(text), opts.onCursor({ line, word }), opts.onSave()
 */
function createEditor(parent, opts = {}) {
  const docs = opts.docs || {};
  const theme = new Compartment();

  const completions = Object.entries(docs).map(([name, d]) => ({ label: name, type: 'keyword', detail: d[0], info: d[1] }));
  const directiveCompletion = ctx => {
    const w = ctx.matchBefore(/^\s*[a-z_]*$/m) && ctx.matchBefore(/[a-z_]*/);
    if (!w || (w.from === w.to && !ctx.explicit)) return null;
    return { from: w.from, options: completions, validFor: /^[a-z_]*$/ };
  };

  const hover = hoverTooltip((view, pos) => {
    const w = wordAt(view.state, pos);
    const d = w && docs[w.word];
    if (!d) return null;
    return {
      pos: w.from, end: w.to, above: true,
      create() {
        const dom = document.createElement('div');
        dom.className = 'np-tooltip';
        const code = document.createElement('code'); code.textContent = d[0];
        const p = document.createElement('div'); p.textContent = d[1];
        const a = document.createElement('a'); a.href = 'https://nginx.org/en/docs/' + d[2]; a.target = '_blank'; a.rel = 'noopener'; a.textContent = 'nginx.org docs';
        dom.append(code, p, a);
        return { dom };
      },
    };
  });

  const lintExt = opts.lint ? linter(async view => {
    const findings = await opts.lint(view.state.doc.toString());
    const doc = view.state.doc;
    return findings.filter(f => f.line >= 1 && f.line <= doc.lines).map(f => {
      const line = doc.line(f.line);
      const indent = line.text.length - line.text.trimStart().length;
      return { from: line.from + indent, to: line.to, severity: f.severity === 'error' ? 'error' : f.severity === 'warning' ? 'warning' : 'info', message: f.message, source: f.rule };
    });
  }, { delay: 600 }) : [];

  const view = new EditorView({
    parent,
    state: EditorState.create({
      doc: '',
      extensions: [
        lineNumbers(), highlightActiveLineGutter(), highlightActiveLine(), drawSelection(), history(),
        indentOnInput(), bracketMatching(), closeBrackets(), highlightSelectionMatches(),
        StreamLanguage.define(nginx), syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        autocompletion({ override: [directiveCompletion] }), hover, lintGutter(), lintExt,
        keymap.of([
          { key: 'Mod-s', preventDefault: true, run: () => { opts.onSave && opts.onSave(); return true; } },
          indentWithTab, ...defaultKeymap, ...historyKeymap, ...searchKeymap, ...completionKeymap,
        ]),
        theme.of(opts.dark === false ? [] : oneDark),
        EditorView.theme({ '&': { height: '100%', fontSize: '12px' }, '.cm-scroller': { fontFamily: 'JetBrains Mono, monospace' } }),
        EditorView.updateListener.of(u => {
          if (u.docChanged && opts.onChange) opts.onChange(u.state.doc.toString());
          if ((u.selectionSet || u.docChanged) && opts.onCursor) {
            const pos = u.state.selection.main.head;
            const line = u.state.doc.lineAt(pos);
            opts.onCursor({ line: line.number, word: (line.text.trim().match(/^([a-z_0-9]+)/) || [])[1] });
          }
        }),
      ],
    }),
  });

  return {
    view,
    get: () => view.state.doc.toString(),
    set: text => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text }, selection: { anchor: 0 } }),
    gotoLine: n => {
      const line = view.state.doc.line(Math.min(Math.max(1, n), view.state.doc.lines));
      view.dispatch({ selection: { anchor: line.from, head: line.to }, scrollIntoView: true });
      view.focus();
    },
    insert: text => {
      const pos = view.state.selection.main.head;
      view.dispatch({ changes: { from: pos, insert: text }, selection: { anchor: pos + text.length }, scrollIntoView: true });
      view.focus();
    },
    relint: () => forceLinting(view),
    setTheme: dark => view.dispatch({ effects: theme.reconfigure(dark ? oneDark : []) }),
  };
}

window.NP = { createEditor, dagre };
