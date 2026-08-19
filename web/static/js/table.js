// Shared table behaviour: per-column filtering, sorting, live count, clear,
// and optional collapsible grouping. Rows expose data-<col> attributes; the
// component reads them from the DOM so templates stay declarative.
document.addEventListener('alpine:init', () => {
  Alpine.data('tableFilter', (keys = [], opts = {}) => ({
    filters: Object.fromEntries(keys.map((k) => [k, ''])),
    sortKey: '',
    sortDir: 1,
    groupKey: opts.groupKey || '', // '' = no grouping, else a column key
    collapsed: {},
    visible: 0,
    total: 0,
    rows: [],
    tbody: null,

    init() {
      this.tbody = this.$el.querySelector('tbody');
      this.rows = Array.from(this.tbody.querySelectorAll('tr[data-row]'));
      this.total = this.rows.length;
      this.refresh();
    },

    matches(r) {
      return Object.entries(this.filters).every(
        ([k, v]) => !v || (r.dataset[k] || '').toLowerCase().includes(String(v).toLowerCase())
      );
    },

    cmp(a, b) {
      const x = a.dataset[this.sortKey] || '';
      const y = b.dataset[this.sortKey] || '';
      const nx = parseFloat(x);
      const ny = parseFloat(y);
      const c = !isNaN(nx) && !isNaN(ny) ? nx - ny : x.localeCompare(y);
      return c * this.sortDir;
    },

    refresh() {
      let ordered = this.rows.slice();
      if (this.groupKey) {
        ordered.sort((a, b) =>
          (a.dataset[this.groupKey] || '').localeCompare(b.dataset[this.groupKey] || '')
        );
      } else if (this.sortKey) {
        ordered.sort((a, b) => this.cmp(a, b));
      }

      this.tbody.querySelectorAll('tr[data-gh]').forEach((e) => e.remove());

      let n = 0;
      let lastG = null;
      for (const r of ordered) {
        const pass = this.matches(r);
        const g = this.groupKey ? r.dataset[this.groupKey] || '' : '';
        if (this.groupKey && pass && g !== lastG) {
          lastG = g;
          const count = ordered.filter(
            (x) => this.matches(x) && (x.dataset[this.groupKey] || '') === g
          ).length;
          this.tbody.appendChild(this.groupHeader(g, count));
        }
        r.style.display = pass && !(this.groupKey && this.collapsed[g]) ? '' : 'none';
        if (pass) n++;
        this.tbody.appendChild(r);
      }
      this.visible = n;
    },

    groupHeader(g, count) {
      const hdr = document.createElement('tr');
      hdr.setAttribute('data-gh', '');
      hdr.className = 'bg-gray-100 dark:bg-gray-700 cursor-pointer select-none';
      const td = document.createElement('td');
      td.colSpan = 20;
      td.className = 'px-3 py-1.5 font-medium text-gray-700 dark:text-gray-200';
      td.textContent = (this.collapsed[g] ? '▸ ' : '▾ ') + g + ' (' + count + ')';
      hdr.appendChild(td);
      hdr.addEventListener('click', () => {
        this.collapsed[g] = !this.collapsed[g];
        this.refresh();
      });
      return hdr;
    },

    sortBy(k) {
      if (this.sortKey === k) this.sortDir *= -1;
      else {
        this.sortKey = k;
        this.sortDir = 1;
      }
      this.groupKey = '';
      this.refresh();
    },

    clear() {
      for (const k in this.filters) this.filters[k] = '';
      this.refresh();
    },
  }));

  // Async enable/disable switch. POSTs the toggle and flips in place (no reload);
  // also updates the row's data-enabled so table filters/sort stay in sync.
  Alpine.data('toggle', (opts) => ({
    on: opts.on,
    busy: false,
    flip(el) {
      if (this.busy) return;
      this.busy = true;
      fetch(opts.url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({ ...opts.params, enabled: String(!this.on) }),
      })
        .then((r) => (r.ok ? r : r.text().then((t) => Promise.reject(t || r.status))))
        .then(() => {
          this.on = !this.on;
          const tr = el.closest('tr');
          if (tr) tr.dataset.enabled = this.on ? opts.onData : opts.offData;
        })
        .catch((e) => alert('Could not change state: ' + e))
        .finally(() => (this.busy = false));
    },
  }));

  // Read-only detail slide-over, shared across pages.
  Alpine.store('drawer', {
    open: false,
    fields: {},
    show(detail) {
      this.fields = detail;
      this.open = true;
    },
    close() {
      this.open = false;
    },
    reveal(path) {
      fetch('/open', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: 'path=' + encodeURIComponent(path),
      });
    },
    viewRaw(path) {
      const modal = document.getElementById('modal');
      fetch('/raw?path=' + encodeURIComponent(path))
        .then((r) => (r.ok ? r.text() : r.text().then((t) => Promise.reject(t))))
        .then((text) => {
          modal.innerHTML =
            '<div class="fixed inset-0 z-50 flex items-center justify-center p-4">' +
            '<div class="absolute inset-0 bg-black/40" data-close></div>' +
            '<div class="relative bg-white dark:bg-gray-800 rounded shadow-xl w-full max-w-3xl max-h-[80vh] overflow-auto">' +
            '<div class="flex items-center justify-between px-4 py-2 border-b border-gray-200 dark:border-gray-700">' +
            '<span class="font-mono text-xs text-gray-500 dark:text-gray-400" data-path></span>' +
            '<button data-close class="text-gray-400 hover:text-gray-700 dark:hover:text-gray-200">&times;</button></div>' +
            '<pre class="p-4 text-xs whitespace-pre-wrap break-words text-gray-800 dark:text-gray-200" data-body></pre></div></div>';
          // textContent (not innerHTML) so file contents can never inject markup.
          modal.querySelector('[data-path]').textContent = path;
          modal.querySelector('[data-body]').textContent = text;
          modal.querySelectorAll('[data-close]').forEach((el) =>
            el.addEventListener('click', () => (modal.innerHTML = ''))
          );
        })
        .catch((err) => alert('Could not read file: ' + err));
    },
  });
});
