<script>
  // Error state for data views (R-27): says what failed, why, and offers the next action.
  let { what, error, onretry } = $props();
</script>

<div class="loaderr" role="alert">
  <strong>{what} konnten nicht geladen werden.</strong>
  <span>{error?.message || error || 'Der Server hat nicht geantwortet.'}</span>
  {#if onretry}<button class="small" onclick={onretry}>Erneut laden</button>{/if}
</div>

<style>
  .loaderr {
    display: flex; flex-direction: column; align-items: flex-start; gap: var(--s-2);
    padding: var(--s-4) var(--s-5); background: var(--error-soft); color: var(--error);
    border: 1px solid var(--error); border-radius: var(--r-1);
  }
  .loaderr span { color: var(--ink-2); font-size: var(--t-sm); overflow-wrap: anywhere; }
</style>
