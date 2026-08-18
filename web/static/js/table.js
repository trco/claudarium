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
  });
});
