<script>
  // Sortable column header (ISO 9241-112 Steuerbarkeit, WAI-ARIA aria-sort).
  let { sorter, key, children, align = 'left' } = $props();
  const active = $derived(sorter.key === key);
</script>

<th class:right={align === 'right'} aria-sort={active ? (sorter.dir === 'asc' ? 'ascending' : 'descending') : undefined}>
  <button class="sort" onclick={() => sorter.toggle(key)}>
    {@render children()}
    <span class="arrow" aria-hidden="true">{active ? (sorter.dir === 'asc' ? '▲' : '▼') : '↕'}</span>
    <span class="sr-only">{active ? (sorter.dir === 'asc' ? ', aufsteigend sortiert' : ', absteigend sortiert') : ', sortieren'}</span>
  </button>
</th>
