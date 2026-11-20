// Client-side table sorting. Usage:
//   const sorter = createSorter('name', { name: (r) => r.name, date: (r) => r.createdAt });
//   sorter.apply(rows)  -> sorted copy;  <SortTh {sorter} key="name">Name</SortTh>
const collator = new Intl.Collator('de', { numeric: true, sensitivity: 'base' });

export function createSorter(key, getters, dir = 'asc', storageKey = '') {
  let initial = { key, dir };
  if (storageKey) {
    try { initial = { ...initial, ...JSON.parse(localStorage.getItem(storageKey) || '{}') }; } catch { /* keep default order */ }
    if (!getters[initial.key]) initial = { key, dir };
  }
  const state = $state(initial);
  return {
    get key() { return state.key; },
    get dir() { return state.dir; },
    toggle(k) {
      if (state.key === k) state.dir = state.dir === 'asc' ? 'desc' : 'asc';
      else { state.key = k; state.dir = 'asc'; }
      if (storageKey) { try { localStorage.setItem(storageKey, JSON.stringify(state)); } catch { /* order applies for this visit only */ } }
    },
    apply(rows) {
      const get = getters[state.key];
      if (!get || !rows) return rows || [];
      const sign = state.dir === 'asc' ? 1 : -1;
      return [...rows].sort((a, b) => {
        const x = get(a), y = get(b);
        if (x == null && y == null) return 0;
        if (x == null) return 1;
        if (y == null) return -1;
        if (typeof x === 'number' && typeof y === 'number') return (x - y) * sign;
        return collator.compare(String(x), String(y)) * sign;
      });
    }
  };
}
